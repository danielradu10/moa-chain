package block

import (
	"bytes"

	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validation"
)

type blockProcessor struct {
	transactionProcessor validation.TxProcessor
	blockchainState      state.BlockchainState
}

func (bp *blockProcessor) ProcessBlock(block *data.Block) error {
	currentBlockHeader, err := bp.blockchainState.CurrentBlockHeader()
	if err != nil {
		return err
	}

	err = bp.validateBlockHeader(&block.Header, currentBlockHeader)
	if err != nil {
		return err
	}

	// validate transactions

	// validate block consumption

	// process block
	return nil
}

func (bp *blockProcessor) validateBlockHeader(
	blockToBeValidated *data.BlockHeader,
	currentBlockHeader *data.BlockHeader,
) error {
	if blockToBeValidated == nil || currentBlockHeader == nil {
		return validation.ErrNilBlock
	}

	// check for nonce continuity
	blockNonce := blockToBeValidated.Nonce
	currentChainNonce := currentBlockHeader.Nonce
	if blockNonce != currentChainNonce+1 {
		return validation.ErrBlockNonceNotContinuous
	}

	// check for round continuity
	blockRound := blockToBeValidated.Round
	currentChainRound := currentBlockHeader.Round
	if blockRound <= currentChainRound {
		return validation.ErrWrongBlockRound
	}

	// check for mini-round continuity
	blockMiniRound := blockToBeValidated.MiniRound
	currentChainMiniRound := currentBlockHeader.MiniRound
	// TODO maybe this code will not be common for all mini-rounds, so we will actually have only one MiniRound check?
	if (validation.MiniRound(blockMiniRound) != validation.MiniRoundOne &&
		validation.MiniRound(blockMiniRound) != validation.MiniRoundTwo &&
		validation.MiniRound(blockMiniRound) != validation.MiniRoundThree) || blockMiniRound != currentChainMiniRound+1 {
		return validation.ErrWrongMiniBlockRound
	}

	// check that the new root hash is constructed over the latest root hash
	blockPreviousRootHash := blockToBeValidated.PreviousRootHash
	currentChainLatestRootHash := currentBlockHeader.RootHash
	if bytes.Compare(blockPreviousRootHash, currentChainLatestRootHash) != 0 {
		return validation.ErrDiscontinuousRootHash
	}

	// check that the new block is constructed over the last block
	blockPreviousHash := blockToBeValidated.PreviousHash
	currentChainHeaderHash := currentBlockHeader.HeaderHash
	if bytes.Compare(currentChainHeaderHash, blockPreviousHash) != 0 {
		return validation.ErrDiscontinuousHash
	}

	return nil
}

func (bp *blockProcessor) processBlock(blockBody *data.BlockBody) error {
	txs := blockBody.Transactions
	for _, tx := range txs {
		// TODO should also check for consumption
		// TODO txs in block should be sent only by hash? should we take the actual tx from mempool
		//  if not present in mempool, from another sync component
		err := bp.transactionProcessor.ProcessTransaction(tx)
		if err != nil {
			return err
		}
	}

	return nil
}
