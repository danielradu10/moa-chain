package validation

import (
	"bytes"

	"moa-chain/agent"
	"moa-chain/blockprocessing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/transactionprocessing"
	"moa-chain/transactionprocessing/processor"
)

type blockProcessor struct {
	accountsSnapshotFactory state.AccountsSnapshotFactory
	blockchainState         state.BlockchainState
	labeler                 agent.Labeler
}

func (bp *blockProcessor) ValidateBlock(block *data.Block) error {
	if block == nil {
		return blockprocessing.ErrNilBlock
	}

	currentBlockHeader, err := bp.blockchainState.CurrentBlockHeader()
	if err != nil {
		return err
	}

	err = bp.validateBlockHeader(&block.Header, currentBlockHeader)
	if err != nil {
		return err
	}

	snapshot, err := bp.accountsSnapshotFactory.CreateSnapshot()
	if err != nil {
		return err
	}
	defer snapshot.Discard()

	txProcessor, err := processor.NewTxProcessor(
		snapshot,
		bp.labeler,
	)
	if err != nil {
		return err
	}

	// validate transactions
	err = bp.validateBlockBody(&block.Body, txProcessor)
	if err != nil {
		return err
	}

	return nil
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
	execResult, err := executor.ExecuteBlockBody(blockBody, transactionProcessor)
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
