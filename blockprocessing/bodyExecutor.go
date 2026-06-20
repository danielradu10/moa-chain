package blockprocessing

import (
	"log/slog"

	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/transactionprocessing"
)

const MaxBlockConsumption = 1000

type bodyExecutor struct {
	logger *slog.Logger
}

func NewBodyExecutor(loggers ...*slog.Logger) *bodyExecutor {
	return &bodyExecutor{
		logger: logging.FromOptional(loggers...),
	}
}

func (exec *bodyExecutor) ExecuteBlockBody(
	blockBody *data.BlockBody,
	transactionProcessor transactionprocessing.TxProcessor,
) (*data.BlockBodyExecutionResult, error) {
	exec.logger.Info("blockprocessing.ExecuteBlockBody started", "numTransactions", len(blockBody.Transactions))

	blockConsumption := uint64(0)
	uniqueTxHashes := make(map[string]struct{})
	labels := map[string][]string{}

	txs := blockBody.Transactions
	for i, tx := range txs {
		txHash := tx.GetTxHash()
		exec.logger.Debug(
			"processing transaction in block body",
			"index", i,
			"txHash", string(txHash),
			"sender", string(tx.GetSender()),
			"nonce", tx.GetNonce(),
			"estimatedConsumption", tx.GetEstimatedConsumption(),
			"estimatedScore", tx.GetEstimatedScore(),
		)

		if _, ok := uniqueTxHashes[string(txHash)]; ok {
			exec.logger.Error("blockprocessing.ExecuteBlockBody duplicated transaction in block body", "txHash", string(txHash))
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(txHash)] = struct{}{}

		if i > 0 {
			err := transactionProcessor.ValidateTransactionsOrdering(txs[i-1], tx)
			if err != nil {
				exec.logger.Error("blockprocessing.ExecuteBlockBody transaction ordering validation failed", "txHash", string(txHash), "error", err)
				return nil, err
			}
		}

		// TODO txs in block should be sent only by hash? should we take the actual tx from mempool
		//  if not present in mempool, from another sync component
		estimatedConsumption, err := transactionProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
		if err != nil {
			exec.logger.Error("blockprocessing.ExecuteBlockBody economic transaction processing failed", "txHash", string(txHash), "error", err)
			return nil, err
		}

		// TODO discuss if this order is ok. should we first label, then process economically?
		labelsGeneratedByMe, err := transactionProcessor.LabelTransaction(tx)
		if err != nil {
			exec.logger.Error("blockprocessing.ExecuteBlockBody transaction labeling failed", "txHash", string(txHash), "error", err)
			return nil, err
		}
		exec.logger.Debug("transaction labeled", "txHash", string(txHash), "labels", labelsGeneratedByMe)

		blockConsumption += estimatedConsumption
		if blockConsumption > MaxBlockConsumption {
			exec.logger.Error(
				"blockprocessing.ExecuteBlockBody block consumption limit exceeded",
				"txHash", string(txHash),
				"blockConsumption", blockConsumption,
				"maxBlockConsumption", MaxBlockConsumption,
			)
			return nil, ErrBlockConsumptionReached
		}

		labels[string(txHash)] = labelsGeneratedByMe
	}

	exec.logger.Info(
		"blockprocessing.ExecuteBlockBody finished",
		"numTransactions", len(txs),
		"totalConsumption", blockConsumption,
		"numLabeledTransactions", len(labels),
	)

	return &data.BlockBodyExecutionResult{
		Transactions:     txs,
		TotalConsumption: blockConsumption,
		Subdomains:       labels,
	}, nil
}
