package validation

import (
	"bytes"

	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/data"
	"moa-chain/transactionprocessing"
	"moa-chain/transactionprocessing/processor"
)

type blockProcessor struct {
	blockprocessing.Base
}

func NewBlockProcessor(base blockprocessing.Base) *blockProcessor {
	return &blockProcessor{
		base,
	}
}

// ValidateBlock validates a proposed block
func (bp *blockProcessor) ValidateBlock(block *data.Block) ([]byte, error) {
	if block == nil {
		return nil, blockprocessing.ErrNilBlock
	}

	currentBlockHeader, err := bp.BlockchainState.CurrentBlockHeader()
	if err != nil {
		return nil, err
	}

	err = bp.validateBlockHeader(&block.Header, currentBlockHeader)
	if err != nil {
		return nil, err
	}

	snapshot, err := bp.AccountsSnapshotFactory.CreateSnapshot()
	if err != nil {
		return nil, err
	}
	defer snapshot.Discard()

	txProcessor, err := processor.NewTxProcessor(
		snapshot,
		bp.AccountState,
		bp.Labeler,
		bp.Mempool,
	)
	if err != nil {
		return nil, err
	}

	// validate transactions
	err = bp.validateBlockBody(&block.Body, txProcessor)
	if err != nil {
		return nil, err
	}

	// TODO validate the new root hash

	hash, err := bp.hashProposedBlock(&block.Body, &block.Header)
	if err != nil {
		return nil, err
	}

	return hash, nil
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

func (bp *blockProcessor) validateBlockBody(blockBody *data.BlockBody, transactionProcessor transactionprocessing.TxProcessor) error {
	executor := blockprocessing.NewBodyExecutor()
	execResult, err := executor.ExecuteBlockBody(blockBody, transactionProcessor, false)
	if err != nil {
		return err
	}

	return bp.validateSubDomains(blockBody.Subdomains, execResult.Subdomains)
}

func (bp *blockProcessor) validateSubDomains(
	subDomainsByLeader map[string]uint64,
	subDomainsByMe map[string]uint64,
) error {
	if len(subDomainsByMe) != len(subDomainsByLeader) {
		return blockprocessing.ErrInvalidNumSubdomains
	}

	for subdomain, freqByLeader := range subDomainsByLeader {
		freqByMe, ok := subDomainsByMe[subdomain]
		if !ok {
			return blockprocessing.ErrInvalidSubdomain
		}

		if freqByMe != freqByLeader {
			return blockprocessing.ErrInvalidFrequencyOfSubdomain
		}
	}

	return nil
}

func (bp *blockProcessor) hashProposedBlock(
	proposedBody *data.BlockBody,
	proposedHeader *data.BlockHeader,
) ([]byte, error) {
	return hashing.ComputeBlockHash(proposedBody, proposedHeader)
}
