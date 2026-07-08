package blockprocessing

import (
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
	logger     *slog.Logger
	batchAgent agent.BatchAgent
}

func NewBodyExecutor(batchAgent agent.BatchAgent, loggers ...*slog.Logger) *bodyExecutor {
	return &bodyExecutor{
		logger:     logging.FromOptional(loggers...),
		batchAgent: batchAgent,
	}
}

// ExecuteBlockBodyMiniRoundOne executes the block body in mini-round one.
// Each transaction is executed economically and then all are labeled via a single batch call.
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

	results, err := exec.batchAgent.LabelBatch(txs)
	if err != nil {
		exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundOne batch label call failed", "error", err)
		return nil, err
	}

	labels := make(map[string][]string, len(results))
	for _, r := range results {
		labels[string(r.TxHash)] = r.Labels
		exec.logger.Debug("transaction labeled (batch)", "txHash", string(r.TxHash), "labels", r.Labels)
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

// ExecuteBlockBodyMiniRoundTwo executes the block in mini-round two.
// All transaction prompts are answered via a single batch call.
func (exec *bodyExecutor) ExecuteBlockBodyMiniRoundTwo(
	blockBody *data.BlockBody,
) (*data.BlockBodyExecutionResultMRTwo, error) {
	if blockBody == nil {
		exec.logger.Error("blockprocessing.ExecuteBlockBodyMiniRoundTwo nil block body")
		return nil, ErrNilBlock
	}

	txs := blockBody.Transactions
	uniqueTxHashes := make(map[string]struct{}, len(txs))

	for _, tx := range txs {
		if tx == nil {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo nil transaction")
			return nil, ErrNilTransaction
		}
		if _, ok := uniqueTxHashes[string(tx.GetTxHash())]; ok {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo duplicated transaction in block body", "txHash", string(tx.GetTxHash()))
			return nil, ErrDuplicatedTransaction
		}
		uniqueTxHashes[string(tx.GetTxHash())] = struct{}{}
	}

	answerResults, err := exec.batchAgent.AnswerBatch(txs)
	if err != nil {
		exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo answer batch call failed", "error", err)
		return nil, err
	}

	txsResults := make([]data.TransactionResult, 0, len(answerResults))
	totalConsumption := uint64(0)

	for _, ar := range answerResults {
		consumption, err := processor.CountTokensFromAnswer(ar.Answer)
		if err != nil {
			exec.logger.Error("bodyExecutor.ExecuteBlockBodyMiniRoundTwo token counting failed", "txHash", string(ar.TxHash), "error", err)
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
