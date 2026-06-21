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

// ExecuteBlockBodyMiniRoundOne executes the block body in mini-round one.
// This means that each transaction is executed economically and labeled.
func (exec *bodyExecutor) ExecuteBlockBodyMiniRoundOne(
	blockBody *data.BlockBody,
	transactionProcessor transactionprocessing.TxProcessor,
) (*data.BlockBodyExecutionResultMROne, error) {
	exec.logger.Info("blockprocessing.ExecuteBlockBodyMiniRoundOne started", "numTransactions", len(blockBody.Transactions))

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
			exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundOne duplicated transaction in block body", "txHash", string(txHash))
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(txHash)] = struct{}{}

		if i > 0 {
			err := transactionProcessor.ValidateTransactionsOrdering(txs[i-1], tx)
			if err != nil {
				exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundOne transaction ordering validation failed", "txHash", string(txHash), "error", err)
				return nil, err
			}
		}

		// TODO txs in block should be sent only by hash? should we take the actual tx from mempool
		//  if not present in mempool, from another sync component
		estimatedConsumption, err := transactionProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
		if err != nil {
			exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundOne economic transaction processing failed", "txHash", string(txHash), "error", err)
			return nil, err
		}

		// TODO discuss if this order is ok. should we first label, then process economically?
		labelsGeneratedByMe, err := transactionProcessor.LabelTransaction(tx)
		if err != nil {
			exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundOne transaction labeling failed", "txHash", string(txHash), "error", err)
			return nil, err
		}
		exec.logger.Debug("transaction labeled", "txHash", string(txHash), "labels", labelsGeneratedByMe)

		blockConsumption += estimatedConsumption
		if blockConsumption > MaxBlockConsumption {
			exec.logger.Error(
				"blockprocessing.ExecuteBlockBodyMiniRoundOne block consumption limit exceeded",
				"txHash", string(txHash),
				"blockConsumption", blockConsumption,
				"maxBlockConsumption", MaxBlockConsumption,
			)
			return nil, ErrBlockConsumptionReached
		}

		labels[string(txHash)] = labelsGeneratedByMe
	}

	exec.logger.Info(
		"blockprocessing.ExecuteBlockBodyMiniRoundOne finished",
		"numTransactions", len(txs),
		"totalConsumption", blockConsumption,
		"numLabeledTransactions", len(labels),
	)

	return &data.BlockBodyExecutionResultMROne{
		Transactions:     txs,
		TotalConsumption: blockConsumption,
		Subdomains:       labels,
	}, nil
}

// ExecuteBlockBodyMiniRoundTwo executes the block in the mini-round two.
// The transactions are already executed economically and verified, so this round only executes the actual prompts.
func (exec *bodyExecutor) ExecuteBlockBodyMiniRoundTwo(
	blockBody *data.BlockBody,
	transactionProcessor transactionprocessing.TxProcessor,
) (*data.BlockBodyExecutionResultMRTwo, error) {
	txsResults := make([]data.TransactionResult, 0, len(blockBody.Transactions))
	totalConsumption := uint64(0)
	for _, tx := range blockBody.Transactions {
		txResult, err := transactionProcessor.ExecutePromptTransaction(tx)
		if err != nil {
			return nil, err
		}

		txsResults = append(txsResults, *txResult)
		totalConsumption += txResult.ActualConsumption
	}

	return &data.BlockBodyExecutionResultMRTwo{
		TxsResults:       txsResults,
		TotalConsumption: totalConsumption,
	}, nil
}
