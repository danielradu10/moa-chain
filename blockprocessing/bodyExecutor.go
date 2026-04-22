package blockprocessing

import (
	"moa-chain/data"
	"moa-chain/transactionprocessing"
)

type blockBodyExecutionResult struct {
	TotalConsumption uint64
	Subdomains       map[string]uint64
}

type bodyExecutor struct {
}

func NewBodyExecutor() *bodyExecutor {
	return &bodyExecutor{}
}

func (exec *bodyExecutor) ExecuteBlockBody(
	blockBody *data.BlockBody,
	transactionProcessor transactionprocessing.TxProcessor,
) (*blockBodyExecutionResult, error) {
	blockConsumption := uint64(0)
	uniqueTxHashes := make(map[string]struct{})
	labelsFrequencies := map[string]uint64{}

	txs := blockBody.Transactions
	for i, tx := range txs {
		txHash := tx.GetTxHash()
		if _, ok := uniqueTxHashes[string(txHash)]; ok {
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(txHash)] = struct{}{}

		if i > 0 {
			err := transactionProcessor.ValidateTransactionsOrdering(txs[i-1], tx)
			if err != nil {
				return nil, err
			}
		}

		// TODO txs in block should be sent only by hash? should we take the actual tx from mempool
		//  if not present in mempool, from another sync component
		estimatedConsumption, err := transactionProcessor.ProcessTransaction(tx, data.MiniRoundOne)
		if err != nil {
			return nil, err
		}

		blockConsumption += estimatedConsumption
		if blockConsumption > MaxBlockConsumption {
			return nil, ErrBlockConsumptionReached
		}

		for _, label := range tx.GetDomainLabels() {
			labelsFrequencies[label]++
		}
	}

	return &blockBodyExecutionResult{
		TotalConsumption: blockConsumption,
		Subdomains:       labelsFrequencies,
	}, nil
}
