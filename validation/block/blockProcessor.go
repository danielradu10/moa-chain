package block

import (
	"bytes"

	"moa-chain/agent"
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
	labeler                 agent.Labeler
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

	txProcessor, err := transaction.NewTxProcessor(
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
	blockConsumption := uint64(0)
	uniqueTxHashes := make(map[string]struct{})
	labelsFrequencies := map[string]uint64{}

	txs := blockBody.Transactions
	for i, tx := range txs {
		txHash := tx.GetTxHash()
		_, ok := uniqueTxHashes[string(txHash)]
		if ok {
			return validation.ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(txHash)] = struct{}{}

		if i > 0 {
			err := bp.validateTransactionsOrdering(txs[i-1], tx)
			if err != nil {
				return err
			}
		}

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

		labels := tx.GetDomainLabels()
		for _, label := range labels {
			_, ok = labelsFrequencies[label]
			if !ok {
				labelsFrequencies[label] = 0
			}

			labelsFrequencies[label] += 1
		}
	}

	return bp.validateSubDomains(blockBody.Subdomains, labelsFrequencies)
}

func (bp *blockProcessor) validateTransactionsOrdering(
	previousTransaction data.Transaction,
	currentTransaction data.Transaction,
) error {
	prevScore := previousTransaction.GetEstimatedScore()
	currScore := currentTransaction.GetEstimatedScore()

	if prevScore < currScore {
		return validation.ErrTxsDoNotRespectProtocolOrder
	}

	if prevScore > currScore {
		return nil
	}

	prevConsumption := previousTransaction.GetEstimatedConsumption()
	currConsumption := currentTransaction.GetEstimatedConsumption()

	if prevConsumption > currConsumption {
		return validation.ErrTxsDoNotRespectProtocolOrder
	}

	if prevConsumption < currConsumption {
		return nil
	}

	if bytes.Compare(previousTransaction.GetTxHash(), currentTransaction.GetTxHash()) > 0 {
		return validation.ErrTxsDoNotRespectProtocolOrder
	}

	return nil
}

func (bp *blockProcessor) validateSubDomains(
	subDomainsByLeader map[string]uint64,
	subDomainsByMe map[string]uint64,
) error {
	if len(subDomainsByMe) != len(subDomainsByLeader) {
		return validation.ErrInvalidNumSubdomains
	}

	for subdomain, freqByLeader := range subDomainsByLeader {
		freqByMe, ok := subDomainsByMe[subdomain]
		if !ok {
			return validation.ErrInvalidSubdomain
		}

		if freqByMe != freqByLeader {
			return validation.ErrInvalidFrequencyOfSubdomain
		}
	}

	return nil
}
