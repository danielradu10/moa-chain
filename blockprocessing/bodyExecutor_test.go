package blockprocessing

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

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
		require.Equal(t, []data.TransactionResult{
			{
				TxHash: []byte("txHash1"),
			},
		}, result.TxsResults)
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
