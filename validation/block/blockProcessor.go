package block

import (
	"bytes"

	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validation"
	"moa-chain/validation/transaction"
)

const (
	maxBlockConsumption = 10000
)

type blockProcessor struct {
	accountsSnapshotFactory state.AccountsSnapshotFactory
	blockchainState         state.BlockchainState
}

func (bp *blockProcessor) ValidateBlock(block *data.Block) error {
	if block == nil {
		return validation.ErrNilBlock
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

	txProcessor, err := transaction.NewTxProcessor(snapshot)
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
		return validation.ErrNilBlock
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
		return validation.ErrBlockNonceNotContinuous
	}

	return nil
}

func (bp *blockProcessor) validateRoundAndMiniRoundContinuity(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.BlockHeader,
) error {
	currentRound := currentBlockHeader.Round
	nextRound := blockToBeValidated.Round

	currentMiniRound := validation.MiniRound(currentBlockHeader.MiniRound)
	nextMiniRound := validation.MiniRound(blockToBeValidated.MiniRound)

	switch currentMiniRound {
	case validation.MiniRoundOne:
		if nextMiniRound != validation.MiniRoundTwo || nextRound != currentRound {
			return validation.ErrWrongMiniBlockRound
		}
	case validation.MiniRoundTwo:
		if nextMiniRound != validation.MiniRoundThree || nextRound != currentRound {
			return validation.ErrWrongMiniBlockRound
		}
	case validation.MiniRoundThree:
		if nextMiniRound != validation.MiniRoundOne || nextRound != currentRound+1 {
			return validation.ErrWrongMiniBlockRound
		}
	default:
		return validation.ErrWrongMiniBlockRound
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
		return validation.ErrDiscontinuousRootHash
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
		return validation.ErrDiscontinuousHash
	}

	return nil
}

func (bp *blockProcessor) validateBlockBody(blockBody *data.BlockBody, transactionProcessor validation.TxProcessor) error {
	txs := blockBody.Transactions
	blockConsumption := uint64(0)
	for _, tx := range txs {
		// TODO should also check for consumption
		// TODO txs in block should be sent only by hash? should we take the actual tx from mempool
		//  if not present in mempool, from another sync component
		estimatedConsumption, err := transactionProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		if err != nil {
			return err
		}

		blockConsumption += estimatedConsumption
		if blockConsumption > maxBlockConsumption {
			return validation.ErrBlockConsumptionReached
		}
	}

	return nil
}
