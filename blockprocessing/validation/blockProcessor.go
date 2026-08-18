package validation

import (
	"bytes"

	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/transactionprocessing"
	"moa-chain/transactionprocessing/processor"
)

type blockProcessor struct {
	blockprocessing.Base
}

func NewBlockProcessor(base blockprocessing.Base) *blockProcessor {
	if base.Logger == nil {
		base.Logger = logging.NewNopLogger()
	}

	return &blockProcessor{
		Base: base,
	}
}

// ValidateBlock validates a proposed block
func (bp *blockProcessor) ValidateBlock(block *data.Block) ([]byte, data.Subdomains, []byte, error) {
	if bp.Logger == nil {
		bp.Logger = logging.NewNopLogger()
	}

	if block == nil {
		bp.Logger.Error("validation.ValidateBlock cannot validate nil block")
		return nil, nil, nil, blockprocessing.ErrNilBlock
	}
	bp.Logger.Info(
		"validation.ValidateBlock started",
		"nonce", block.Header.Nonce,
		"round", block.Header.Round,
		"miniRound", block.Header.MiniRound,
		"epoch", block.Header.Epoch,
		"numTransactions", len(block.Body.Transactions),
	)

	currentBlockHeader, err := bp.BlockchainState.CurrentBlockHeader()
	if err != nil {
		bp.Logger.Error("validation.ValidateBlock failed to load current block header", "error", err)
		return nil, nil, nil, err
	}

	currentMiniRoundRaw, err := bp.BlockchainState.CurrentMiniRound()
	if err != nil {
		bp.Logger.Error("validation.ValidateBlock failed to load current mini-round", "error", err)
		return nil, nil, nil, err
	}
	currentMiniRound := data.MiniRound(currentMiniRoundRaw)

	err = bp.validateBlockHeader(&block.Header, currentBlockHeader, currentMiniRound)
	if err != nil {
		bp.Logger.Error("validation.ValidateBlock block header validation failed", "error", err)
		return nil, nil, nil, err
	}
	bp.Logger.Debug("block header validation passed")

	snapshot, err := bp.AccountsSnapshotFactory.CreateSnapshot()
	if err != nil {
		bp.Logger.Error("validation.ValidateBlock failed to create accounts snapshot", "error", err)
		return nil, nil, nil, err
	}
	defer snapshot.Discard()

	txProcessor, err := processor.NewTxProcessor(
		snapshot,
		bp.AccountState,
		bp.Mempool,
	)
	if err != nil {
		bp.Logger.Error("failed to create transaction processor for validation", "error", err)
		return nil, nil, nil, err
	}

	// validate transactions and generate labels for each transaction.
	subdomains, err := bp.validateAndExecuteBlockBody(&block.Body, txProcessor)
	if err != nil {
		bp.Logger.Error("validation.ValidateBlock block body validation or execution failed", "error", err)
		return nil, nil, nil, err
	}

	// TODO validate the new root blockHash

	blockHash, err := bp.hashProposedBlock(&block.Body, &block.Header)
	if err != nil {
		bp.Logger.Error("failed to hash validated block", "error", err)
		return nil, nil, nil, err
	}

	subdomainsHash, err := bp.hashSubdomains(subdomains)
	if err != nil {
		bp.Logger.Error("failed to hash validator subdomains", "error", err)
		return nil, nil, nil, err
	}
	bp.Logger.Info(
		"validation.ValidateBlock finished",
		"numTransactions", len(block.Body.Transactions),
		"numSubdomainMaps", len(subdomains),
		"blockHashLen", len(blockHash),
		"subdomainsHashLen", len(subdomainsHash),
	)

	return blockHash, subdomains, subdomainsHash, nil
}

func (bp *blockProcessor) validateBlockHeader(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.ChainBlockHeader,
	currentMiniRound data.MiniRound,
) error {
	if blockToBeValidated == nil || currentBlockHeader == nil {
		return blockprocessing.ErrNilBlock
	}

	err := bp.validateNonceContinuity(blockToBeValidated, currentBlockHeader)
	if err != nil {
		return err
	}

	err = bp.validateRoundAndMiniRoundContinuity(blockToBeValidated, currentMiniRound, currentBlockHeader.Round)
	if err != nil {
		return err
	}

	err = bp.validateRootHashContinuity(blockToBeValidated, currentBlockHeader)
	if err != nil {
		return err
	}

	err = bp.validateHashContinuity(blockToBeValidated, currentBlockHeader)
	if err != nil {
		return err
	}

	return nil
}

func (bp *blockProcessor) validateNonceContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.ChainBlockHeader,
) error {
	blockNonce := blockToBeValidated.Nonce
	currentChainNonce := currentBlockHeader.Nonce
	if blockNonce != currentChainNonce+1 {
		return blockprocessing.ErrBlockNonceNotContinuous
	}

	return nil
}

func (bp *blockProcessor) validateRoundAndMiniRoundContinuity(
	blockToBeValidated *data.BlockHeader,
	currentMiniRound data.MiniRound,
	currentRound uint64,
) error {
	nextRound := blockToBeValidated.Round
	nextMiniRound := data.MiniRound(blockToBeValidated.MiniRound)

	switch currentMiniRound {
	case data.MiniRoundOne:
		if nextMiniRound != data.MiniRoundTwo || nextRound != currentRound {
			return blockprocessing.ErrWrongMiniBlockRound
		}
	case data.MiniRoundTwo:
		if nextMiniRound != data.MiniRoundThree || nextRound != currentRound {
			return blockprocessing.ErrWrongMiniBlockRound
		}
	case data.MiniRoundThree:
		if nextMiniRound != data.MiniRoundOne || nextRound != currentRound+1 {
			return blockprocessing.ErrWrongMiniBlockRound
		}
	default:
		return blockprocessing.ErrWrongMiniBlockRound
	}

	return nil
}

func (bp *blockProcessor) validateRootHashContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.ChainBlockHeader,
) error {
	blockPreviousRootHash := blockToBeValidated.PreviousRootHash
	currentChainLatestRootHash := currentBlockHeader.RootHash
	if !bytes.Equal(blockPreviousRootHash, currentChainLatestRootHash) {
		return blockprocessing.ErrDiscontinuousRootHash
	}

	return nil
}

func (bp *blockProcessor) validateHashContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.ChainBlockHeader,
) error {
	blockPreviousHash := blockToBeValidated.PreviousHash
	currentChainHeaderHash := currentBlockHeader.HeaderHash
	if !bytes.Equal(currentChainHeaderHash, blockPreviousHash) {
		return blockprocessing.ErrDiscontinuousHash
	}

	return nil
}

func (bp *blockProcessor) validateAndExecuteBlockBody(blockBody *data.BlockBody, transactionProcessor transactionprocessing.TxProcessor) (data.Subdomains, error) {
	executor := blockprocessing.NewBodyExecutor(bp.Store, bp.Logger)
	execResult, err := executor.ExecuteBlockBodyMiniRoundOne(blockBody, transactionProcessor)
	if err != nil {
		return nil, err
	}

	return execResult.Subdomains, nil
}

func (bp *blockProcessor) hashProposedBlock(proposedBody *data.BlockBody, proposedHeader *data.BlockHeader) ([]byte, error) {
	// ComputeBlockHash writes header.BodyHash as a side effect. The broadcaster
	// sends the same *ProposedBlockMessage pointer to every receiver, so multiple
	// goroutines would race on that write. Copy the header to keep the mutation local.
	headerCopy := *proposedHeader
	return hashing.ComputeBlockHash(proposedBody, &headerCopy)
}

func (bp *blockProcessor) hashSubdomains(subdomains data.Subdomains) ([]byte, error) {
	return hashing.ComputeSubdomainsHash(subdomains)
}

// ExecuteBlockPrompts executes the prompts of a block.
func (bp *blockProcessor) ExecuteBlockPrompts(block *data.BlockBody) (*data.BlockBodyExecutionResultMRTwo, error) {
	if bp.Logger == nil {
		bp.Logger = logging.NewNopLogger()
	}

	executor := blockprocessing.NewBodyExecutor(bp.Store, bp.Logger)
	return executor.ExecuteBlockBodyMiniRoundTwo(block)
}
