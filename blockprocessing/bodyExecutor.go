package blockprocessing

import (
	"bytes"
	"log/slog"

	"moa-chain/blockprocessing/hashing"
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
// TODO strengthen this method, the checks in the blockBody and the information returned.
func (exec *bodyExecutor) ExecuteBlockBodyMiniRoundTwo(
	blockBody *data.BlockBody,
	promptExecutor transactionprocessing.PromptExecutor,
) (*data.BlockBodyExecutionResultMRTwo, error) {
	if blockBody == nil {
		exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundTwo nil block body")
		return nil, ErrNilBlock
	}
	if promptExecutor == nil {
		exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo nil prompt executor")
		return nil, ErrNilPromptExecutor
	}

	txsResults := make([]data.TransactionResult, 0, len(blockBody.Transactions))
	totalConsumption := uint64(0)
	uniqueTxHashes := make(map[string]struct{})
	for _, tx := range blockBody.Transactions {
		if tx == nil {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo nil transaction")
			return nil, ErrNilTransaction
		}

		_, ok := uniqueTxHashes[string(tx.GetTxHash())]
		if ok {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo duplicated transaction in block body", "txHash", string(tx.GetTxHash()))
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(tx.GetTxHash())] = struct{}{}

		txResult, err := promptExecutor.ExecutePromptTransaction(tx)
		if err != nil {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo prompt executor failed", "txHash", string(tx.GetTxHash()), "error", err)
			return nil, err
		}
		if txResult == nil {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo nil transaction result", "txHash", string(tx.GetTxHash()))
			return nil, ErrNilTransactionResult
		}

		if !bytes.Equal(txResult.TxHash, tx.GetTxHash()) {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo tx hash mismatch", "txHash", string(tx.GetTxHash()), "txResult.txHash", string(txResult.TxHash))
			return nil, ErrTxHashMismatch
		}

		txsResults = append(txsResults, *txResult)
		totalConsumption += txResult.ActualConsumption
	}

	exec.logger.Info("BlockBody executed in MiniRoundTwo", "totalConsumption", totalConsumption)

	executionResult := &data.BlockBodyExecutionResultMRTwo{
		TxsResults:       txsResults,
		TotalConsumption: totalConsumption,
	}
	blockHash, err := hashing.ComputePromptExecutionHash(executionResult)
	if err != nil {
		exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo failed to hash executed prompt block", "error", err)
		return nil, err
	}
	executionResult.BlockHash = blockHash

	return executionResult, nil
}
