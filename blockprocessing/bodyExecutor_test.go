package blockprocessing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/txpipeline"
)

func TestBodyExecutor_ExecuteBlockBodyMiniRoundOne(t *testing.T) {
	t.Parallel()

	t.Run("should read labels from store", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash2")

		store := txpipeline.NewPrecomputedStore()
		store.StoreLabels([]byte("txHash1"), []string{"databases"})
		store.StoreLabels([]byte("txHash2"), []string{"security", "back_end"})

		result, err := NewBodyExecutor(store).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
			&testscommon.TxProcessorStub{},
		)

		require.NoError(t, err)
		require.Equal(t, []string{"databases"}, result.Subdomains["txHash1"])
		require.Equal(t, []string{"security", "back_end"}, result.Subdomains["txHash2"])
	})

	t.Run("should return ErrMissingPrecomputedLabels when labels not in store", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutor(txpipeline.NewPrecomputedStore()).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			&testscommon.TxProcessorStub{},
		)

		require.Nil(t, result)
		require.Equal(t, ErrMissingPrecomputedLabels, err)
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

		store := txpipeline.NewPrecomputedStore()
		store.StoreLabels([]byte("txHash1"), []string{"databases"})

		result, err := NewBodyExecutor(store).ExecuteBlockBodyMiniRoundOne(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
			txProcessor,
		)

		require.NoError(t, err)
		require.True(t, economicCalled)
		require.Equal(t, uint64(10), result.TotalConsumption)
	})
}

func TestBodyExecutor_ExecuteBlockBodyMiniRoundTwo(t *testing.T) {
	t.Parallel()

	t.Run("should read answers from store and compute token consumption", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash2")

		store := txpipeline.NewPrecomputedStore()
		store.StoreAnswer([]byte("txHash1"), "solution one")
		store.StoreAnswer([]byte("txHash2"), "solution two")

		result, err := NewBodyExecutor(store).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
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

	t.Run("should return ErrMissingPrecomputedAnswer when answer not in store", func(t *testing.T) {
		t.Parallel()

		tx := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutor(txpipeline.NewPrecomputedStore()).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx}},
		)

		require.Nil(t, result)
		require.Equal(t, ErrMissingPrecomputedAnswer, err)
	})

	t.Run("should return ErrNilBlock when block body is nil", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutor(txpipeline.NewPrecomputedStore()).ExecuteBlockBodyMiniRoundTwo(nil)

		require.Nil(t, result)
		require.Equal(t, ErrNilBlock, err)
	})

	t.Run("should return ErrNilTransaction when block contains nil transaction", func(t *testing.T) {
		t.Parallel()

		result, err := NewBodyExecutor(txpipeline.NewPrecomputedStore()).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{nil}},
		)

		require.Nil(t, result)
		require.Equal(t, ErrNilTransaction, err)
	})

	t.Run("should return ErrDuplicatedTransaction when block contains duplicate transaction hashes", func(t *testing.T) {
		t.Parallel()

		tx1 := createBodyExecutorTestTransaction("txHash1")
		tx2 := createBodyExecutorTestTransaction("txHash1")

		result, err := NewBodyExecutor(txpipeline.NewPrecomputedStore()).ExecuteBlockBodyMiniRoundTwo(
			&data.BlockBody{Transactions: []data.Transaction{tx1, tx2}},
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
