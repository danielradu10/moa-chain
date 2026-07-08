package blockprocessing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/testscommon"
)

func TestBodyExecutor_ExecuteBlockBodyMiniRoundTwo(t *testing.T) {
	t.Parallel()

	t.Run("should execute prompts and return transaction results", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash2")
		txProcessor := &testscommon.TxProcessorStub{
			ExecutePromptTransactionCalled: func(tx data.Transaction) (*data.TransactionResult, error) {
				switch string(tx.GetTxHash()) {
				case "txHash1":
					return createTransactionResult(tx, "answer one", 3), nil
				case "txHash2":
					return createTransactionResult(tx, "answer two", 5), nil
				default:
					return createTransactionResult(tx, "unknown answer", 0), nil
				}
			},
		}

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			txProcessor,
		)

		require.NoError(t, err)
		require.Equal(t, uint64(8), result.TotalConsumption)
		require.NotEmpty(t, result.BlockHash)
		require.Equal(t, []data.TransactionResult{
			{
				TxHash:            []byte("txHash1"),
				Answer:            "answer one",
				ActualConsumption: 3,
			},
			{
				TxHash:            []byte("txHash2"),
				Answer:            "answer two",
				ActualConsumption: 5,
			},
		}, result.TxsResults)
	})

	t.Run("should return execution error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("prompt execution error")
		tx := createBodyExecutorTestTransaction("txHash1")
		txProcessor := &testscommon.TxProcessorStub{
			ExecutePromptTransactionCalled: func(tx data.Transaction) (*data.TransactionResult, error) {
				return nil, expectedErr
			},
		}

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			txProcessor,
		)

		require.Nil(t, result)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should use safe default transaction processor stub result", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			&testscommon.TxProcessorStub{},
		)

		require.NoError(t, err)
		require.Equal(t, uint64(0), result.TotalConsumption)
		require.NotEmpty(t, result.BlockHash)
		require.Equal(t, []data.TransactionResult{
			{
				TxHash: []byte("txHash1"),
			},
		}, result.TxsResults)
	})

	t.Run("should return ErrNilBlock when block body is nil", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(nil, &testscommon.TxProcessorStub{})

		require.Nil(t, result)
		require.Equal(t, ErrNilBlock, err)
	})

	t.Run("should return ErrNilPromptExecutor when prompt executor is nil", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(&data.BlockBody{}, nil)

		require.Nil(t, result)
		require.Equal(t, ErrNilPromptExecutor, err)
	})

	t.Run("should return ErrNilTransaction when block contains nil transaction", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{nil}},
			&testscommon.TxProcessorStub{},
		)

		require.Nil(t, result)
		require.Equal(t, ErrNilTransaction, err)
	})

	t.Run("should return ErrDuplicatedTransaction when block contains duplicate transaction hashes", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			&testscommon.TxProcessorStub{},
		)

		require.Nil(t, result)
		require.Equal(t, ErrDuplicatedTransaction, err)
	})

	t.Run("should return ErrNilTransactionResult when prompt executor returns nil result", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")
		txProcessor := &testscommon.TxProcessorStub{
			ExecutePromptTransactionCalled: func(tx data.Transaction) (*data.TransactionResult, error) {
				return nil, nil
			},
		}

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			txProcessor,
		)

		require.Nil(t, result)
		require.Equal(t, ErrNilTransactionResult, err)
	})

	t.Run("should return ErrTxHashMismatch when prompt result has different transaction hash", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")
		txProcessor := &testscommon.TxProcessorStub{
			ExecutePromptTransactionCalled: func(tx data.Transaction) (*data.TransactionResult, error) {
				return &data.TransactionResult{
					TxHash: []byte("differentTxHash"),
				}, nil
			},
		}

		result, err := NewBodyExecutor().ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			txProcessor,
		)

		require.Nil(t, result)
		require.Equal(t, ErrTxHashMismatch, err)
	})
}

func TestBodyExecutor_ExecuteBlockBodyMiniRoundOne_WithBatchAgent(t *testing.T) {
	t.Parallel()

	t.Run("should call LabelBatch instead of per-tx LabelTransaction", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash2")

		labelBatchCalled := false
		perTxLabelCalled := false

		batchAgent := &testscommon.LabelerStub{
			LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
				labelBatchCalled = true
				return []agent.LabelResult{
					{TxHash: []byte("txHash1"), Labels: []string{"databases"}},
					{TxHash: []byte("txHash2"), Labels: []string{"security", "back_end"}},
				}, nil
			},
		}
		txProcessor := &testscommon.TxProcessorStub{
			LabelTransactionCalled: func(tx data.Transaction) ([]string, error) {
				perTxLabelCalled = true
				return nil, nil
			},
		}

		result, err := NewBodyExecutorWithBatchAgent(batchAgent).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			txProcessor,
		)

		require.NoError(t, err)
		require.True(t, labelBatchCalled)
		require.False(t, perTxLabelCalled)
		require.Equal(t, []string{"databases"}, result.Subdomains["txHash1"])
		require.Equal(t, []string{"security", "back_end"}, result.Subdomains["txHash2"])
	})

	t.Run("should propagate LabelBatch error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("label batch failed")
		tx := createBodyExecutorTestTransaction("txHash1")
		batchAgent := &testscommon.LabelerStub{
			LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
				return nil, expectedErr
			},
		}

		result, err := NewBodyExecutorWithBatchAgent(batchAgent).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			&testscommon.TxProcessorStub{},
		)

		require.Nil(t, result)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should still run economic processing per-transaction", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")
		economicCalled := false

		txProcessor := &testscommon.TxProcessorStub{
			ProcessTransactionCalled: func(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
				economicCalled = true
				return 10, nil
			},
		}
		batchAgent := &testscommon.LabelerStub{
			LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
				return []agent.LabelResult{{TxHash: tx.GetTxHash(), Labels: []string{"databases"}}}, nil
			},
		}

		result, err := NewBodyExecutorWithBatchAgent(batchAgent).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			txProcessor,
		)

		require.NoError(t, err)
		require.True(t, economicCalled)
		require.Equal(t, uint64(10), result.TotalConsumption)
	})
}

func TestBodyExecutor_ExecuteBlockBodyMiniRoundTwo_WithBatchAgent(t *testing.T) {
	t.Parallel()

	t.Run("should call AnswerBatch and compute token consumption", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash2")

		batchAgent := &testscommon.LabelerStub{
			AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
				return []agent.AnswerResult{
					{TxHash: []byte("txHash1"), Answer: "solution one"},
					{TxHash: []byte("txHash2"), Answer: "solution two"},
				}, nil
			},
		}

		// promptExecutor is nil — the batch path does not require it.
		result, err := NewBodyExecutorWithBatchAgent(batchAgent).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			nil,
		)

		require.NoError(t, err)
		require.NotEmpty(t, result.BlockHash)
		require.Len(t, result.TxsResults, 2)
		require.Equal(t, []byte("txHash1"), result.TxsResults[0].TxHash)
		require.Equal(t, "solution one", result.TxsResults[0].Answer)
		require.Equal(t, []byte("txHash2"), result.TxsResults[1].TxHash)
		require.Equal(t, "solution two", result.TxsResults[1].Answer)
		require.Greater(t, result.TxsResults[0].ActualConsumption, uint64(0))
		require.Greater(t, result.TotalConsumption, uint64(0))
	})

	t.Run("should propagate AnswerBatch error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("answer batch failed")
		tx := createBodyExecutorTestTransaction("txHash1")
		batchAgent := &testscommon.LabelerStub{
			AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
				return nil, expectedErr
			},
		}

		result, err := NewBodyExecutorWithBatchAgent(batchAgent).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			nil,
		)

		require.Nil(t, result)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should return ErrNilBlock when block body is nil", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutorWithBatchAgent(&testscommon.LabelerStub{}).ExecuteBlockBodyMiniRoundTwo(nil, nil)

		require.Nil(t, result)
		require.Equal(t, ErrNilBlock, err)
	})

	t.Run("should return ErrNilTransaction when block contains nil transaction", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutorWithBatchAgent(&testscommon.LabelerStub{}).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{nil}},
			nil,
		)

		require.Nil(t, result)
		require.Equal(t, ErrNilTransaction, err)
	})

	t.Run("should return ErrDuplicatedTransaction when block contains duplicate transaction hashes", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutorWithBatchAgent(&testscommon.LabelerStub{}).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			nil,
		)

		require.Nil(t, result)
		require.Equal(t, ErrDuplicatedTransaction, err)
	})
}

func createBodyExecutorTestTransaction(txHash string) data.Transaction {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(txHash))
	return tx
}

func createTransactionResult(tx data.Transaction, answer string, consumption uint64) *data.TransactionResult {
	return &data.TransactionResult{
		TxHash:            tx.GetTxHash(),
		Answer:            answer,
		ActualConsumption: consumption,
	}
}
