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
		base,
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

	err = bp.validateBlockHeader(&block.Header, currentBlockHeader)
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
		bp.Labeler,
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
	currentBlockHeader *data.BlockHeader,
) error {
	if blockToBeValidated == nil || currentBlockHeader == nil {
		return blockprocessing.ErrNilBlock
	}

	err := bp.validateNonceContinuity(blockToBeValidated, currentBlockHeader)
	if err != nil {
		return err
	}

	err = bp.validateRoundAndMiniRoundContinuity(blockToBeValidated, currentBlockHeader)
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
	currentBlockHeader *data.BlockHeader,
) error {
	// check for nonce continuity
	blockNonce := blockToBeValidated.Nonce
	currentChainNonce := currentBlockHeader.Nonce
	if blockNonce != currentChainNonce+1 {
		return blockprocessing.ErrBlockNonceNotContinuous
	}

	return nil
}

func (bp *blockProcessor) validateRoundAndMiniRoundContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.BlockHeader,
) error {
	currentRound := currentBlockHeader.Round
	nextRound := blockToBeValidated.Round

	currentMiniRound := data.MiniRound(currentBlockHeader.MiniRound)
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
	currentBlockHeader *data.BlockHeader,
) error {
	// check that the new root hash is constructed over the latest root hash
	blockPreviousRootHash := blockToBeValidated.PreviousRootHash
	currentChainLatestRootHash := currentBlockHeader.RootHash
	if bytes.Compare(blockPreviousRootHash, currentChainLatestRootHash) != 0 {
		return blockprocessing.ErrDiscontinuousRootHash
	}

	return nil
}

func (bp *blockProcessor) validateHashContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.BlockHeader,
) error {
	// check that the new block is constructed over the last block
	blockPreviousHash := blockToBeValidated.PreviousHash
	currentChainHeaderHash := currentBlockHeader.HeaderHash
	if bytes.Compare(currentChainHeaderHash, blockPreviousHash) != 0 {
		return blockprocessing.ErrDiscontinuousHash
	}

	return nil
}

func (bp *blockProcessor) validateAndExecuteBlockBody(blockBody *data.BlockBody, transactionProcessor transactionprocessing.TxProcessor) (data.Subdomains, error) {
	executor := blockprocessing.NewBodyExecutor(bp.Logger)
	execResult, err := executor.ExecuteBlockBody(blockBody, transactionProcessor)
	if err != nil {
		return nil, err
	}

	return execResult.Subdomains, nil
}

func (bp *blockProcessor) hashProposedBlock(proposedBody *data.BlockBody, proposedHeader *data.BlockHeader) ([]byte, error) {
	return hashing.ComputeBlockHash(proposedBody, proposedHeader)
}

func (bp *blockProcessor) hashSubdomains(subdomains data.Subdomains) ([]byte, error) {
	return hashing.ComputeSubdomainsHash(subdomains)
}
