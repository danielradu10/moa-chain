package blockprocessing

import (
	"bytes"
	"log/slog"

	"moa-chain/agent"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/transactionprocessing"
	"moa-chain/transactionprocessing/processor"
)

const MaxBlockConsumption = 1000

type bodyExecutor struct {
	logger *slog.Logger

	// batchAgent is optional. When set, LabelBatch / AnswerBatch are used instead
	// of the per-transaction loop, sending all transactions to the Python service
	// in a single HTTP call. When nil, the existing per-tx path is used (backward
	// compatible with all existing tests and stubs).
	batchAgent agent.BatchAgent
}

func NewBodyExecutor(loggers ...*slog.Logger) *bodyExecutor {
	return &bodyExecutor{
		logger: logging.FromOptional(loggers...),
	}
}

// NewBodyExecutorWithBatchAgent creates a bodyExecutor that uses the given
// BatchAgent for bulk label and answer calls (wired in PR 9 via the HTTP client).
func NewBodyExecutorWithBatchAgent(batchAgent agent.BatchAgent, loggers ...*slog.Logger) *bodyExecutor {
	return &bodyExecutor{
		logger:     logging.FromOptional(loggers...),
		batchAgent: batchAgent,
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
	}

	// Label all transactions — either in one batch call or per-tx depending on
	// whether a BatchAgent is configured.
	labels, err := exec.labelTransactions(txs, transactionProcessor)
	if err != nil {
		return nil, err
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

// labelTransactions returns a map of tx_hash → labels using either the batch
// agent (single HTTP call) or the per-tx TxProcessor path (legacy).
func (exec *bodyExecutor) labelTransactions(
	txs []data.Transaction,
	transactionProcessor transactionprocessing.TxProcessor,
) (map[string][]string, error) {
	labels := make(map[string][]string, len(txs))

	if exec.batchAgent != nil {
		// Batch path: one HTTP call to the Python /label endpoint for all txs.
		results, err := exec.batchAgent.LabelBatch(txs)
		if err != nil {
			exec.logger.Error("blockprocessing.labelTransactions batch label call failed", "error", err)
			return nil, err
		}

		for _, r := range results {
			labels[string(r.TxHash)] = r.Labels
			exec.logger.Debug("transaction labeled (batch)", "txHash", string(r.TxHash), "labels", r.Labels)
		}

		return labels, nil
	}

	// Per-tx path: call LabelTransaction for each transaction individually.
	// Used when no BatchAgent is configured (all existing tests and stubs).
	for _, tx := range txs {
		txHash := tx.GetTxHash()
		labelsGeneratedByMe, err := transactionProcessor.LabelTransaction(tx)
		if err != nil {
			exec.logger.Error("blockprocessing.labelTransactions per-tx label call failed", "txHash", string(txHash), "error", err)
			return nil, err
		}

		exec.logger.Debug("transaction labeled", "txHash", string(txHash), "labels", labelsGeneratedByMe)
		labels[string(txHash)] = labelsGeneratedByMe
	}

	return labels, nil
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

	if exec.batchAgent != nil {
		// Batch path: one HTTP call to the Python /answer endpoint for all txs.
		return exec.executeBlockBodyMiniRoundTwoBatch(blockBody)
	}

	// Per-tx path (legacy): nil check only applies here since batch path
	// does not use promptExecutor.
	if promptExecutor == nil {
		exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo nil prompt executor")
		return nil, ErrNilPromptExecutor
	}

	return exec.executeBlockBodyMiniRoundTwoPerTx(blockBody, promptExecutor)
}

// executeBlockBodyMiniRoundTwoPerTx is the original per-transaction answer path,
// kept for backward compatibility with all existing tests and stubs.
func (exec *bodyExecutor) executeBlockBodyMiniRoundTwoPerTx(
	blockBody *data.BlockBody,
	promptExecutor transactionprocessing.PromptExecutor,
) (*data.BlockBodyExecutionResultMRTwo, error) {
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

	return exec.buildMiniRoundTwoResult(txsResults, totalConsumption)
}

// executeBlockBodyMiniRoundTwoBatch uses BatchAgent.AnswerBatch to fetch all
// answers in a single HTTP call, then computes token consumption for each answer.
func (exec *bodyExecutor) executeBlockBodyMiniRoundTwoBatch(
	blockBody *data.BlockBody,
) (*data.BlockBodyExecutionResultMRTwo, error) {
	txs := blockBody.Transactions
	uniqueTxHashes := make(map[string]struct{}, len(txs))

	// Deduplication check before sending the batch.
	for _, tx := range txs {
		if tx == nil {
			exec.logger.Error("bodyExecutor.executeBlockBodyMiniRoundTwoBatch nil transaction")
			return nil, ErrNilTransaction
		}
		if _, ok := uniqueTxHashes[string(tx.GetTxHash())]; ok {
			exec.logger.Error("bodyExecutor.executeBlockBodyMiniRoundTwoBatch duplicated transaction", "txHash", string(tx.GetTxHash()))
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(tx.GetTxHash())] = struct{}{}
	}

	// One HTTP call to the Python /answer endpoint for all transactions.
	answerResults, err := exec.batchAgent.AnswerBatch(txs)
	if err != nil {
		exec.logger.Error("bodyExecutor.executeBlockBodyMiniRoundTwoBatch answer batch call failed", "error", err)
		return nil, err
	}

	txsResults := make([]data.TransactionResult, 0, len(answerResults))
	totalConsumption := uint64(0)

	for _, ar := range answerResults {
		consumption, err := processor.CountTokensFromAnswer(ar.Answer)
		if err != nil {
			exec.logger.Error("bodyExecutor.executeBlockBodyMiniRoundTwoBatch token counting failed", "txHash", string(ar.TxHash), "error", err)
			return nil, err
		}

		txsResults = append(txsResults, data.TransactionResult{
			TxHash:            ar.TxHash,
			Answer:            ar.Answer,
			ActualConsumption: consumption,
		})
		totalConsumption += consumption
	}

	return exec.buildMiniRoundTwoResult(txsResults, totalConsumption)
}

func (exec *bodyExecutor) buildMiniRoundTwoResult(
	txsResults []data.TransactionResult,
	totalConsumption uint64,
) (*data.BlockBodyExecutionResultMRTwo, error) {
	exec.logger.Info("BlockBody executed in MiniRoundTwo", "totalConsumption", totalConsumption)

	executionResult := &data.BlockBodyExecutionResultMRTwo{
		TxsResults:       txsResults,
		TotalConsumption: totalConsumption,
	}

	blockHash, err := hashing.ComputePromptExecutionHash(executionResult)
	if err != nil {
		exec.logger.Error("bodyExecutor.buildMiniRoundTwoResult failed to hash executed prompt block", "error", err)
		return nil, err
	}
	executionResult.BlockHash = blockHash

	return executionResult, nil
}
