package mempool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemPool_AddTransaction(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilTransaction in case of nil transaction", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		err := mempool.AddTransaction(nil)
		require.Equal(t, ErrNilTransaction, err)
	})

	t.Run("should add transaction", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		err := mempool.AddTransaction(&transaction{
			nonce:                0,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)
		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())
	})

	t.Run("should add two transactions from same sender and keep one address", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		err := mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 20,
			txHash:               []byte("txHash2"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                0,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)

		require.Equal(t, uint64(2), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, uint64(0), aliceTxList.getTxByIndex(0).GetNonce())
		require.Equal(t, uint64(1), aliceTxList.getTxByIndex(1).GetNonce())
	})

	t.Run("should add transactions from different senders", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		err := mempool.AddTransaction(&transaction{
			nonce:                0,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                0,
			sender:               []byte("bob"),
			estimatedConsumption: 15,
			txHash:               []byte("txHash2"),
		})
		require.NoError(t, err)

		require.Equal(t, uint64(2), mempool.NumTransactions())
		require.Equal(t, uint64(2), mempool.NumAddresses())
	})

	t.Run("should not overwrite transaction by hash in case of duplicate tx hash", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		tx1 := &transaction{
			nonce:                0,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("sameHash"),
		}
		tx2 := &transaction{
			nonce:                1,
			sender:               []byte("bob"),
			estimatedConsumption: 20,
			txHash:               []byte("sameHash"),
		}

		err := mempool.AddTransaction(tx1)
		require.NoError(t, err)

		err = mempool.AddTransaction(tx2)
		require.NoError(t, err)

		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, tx1, mempool.transactionsByHash["sameHash"])
	})

	t.Run("should not add transactions to sender list when tx hash is duplicate", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		tx1 := &transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("sameHash"),
		}
		tx2 := &transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("sameHash"),
		}

		err := mempool.AddTransaction(tx1)
		require.NoError(t, err)

		err = mempool.AddTransaction(tx2)
		require.NoError(t, err)

		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 1, len(aliceTxList.transactionList))
	})

	t.Run("should keep sender transactions sorted", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		err := mempool.AddTransaction(&transaction{
			nonce:                2,
			sender:               []byte("alice"),
			estimatedConsumption: 30,
			txHash:               []byte("txHash3"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 20,
			txHash:               []byte("txHash2"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                0,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 3, aliceTxList.numTransactions())
		require.Equal(t, uint64(0), aliceTxList.getTxByIndex(0).GetNonce())
		require.Equal(t, uint64(1), aliceTxList.getTxByIndex(1).GetNonce())
		require.Equal(t, uint64(2), aliceTxList.getTxByIndex(2).GetNonce())
	})

	t.Run("should sort same nonce transactions by estimated consumption", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		err := mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 30,
			txHash:               []byte("txHash2"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, uint64(10), aliceTxList.getTxByIndex(0).GetEstimatedConsumption())
		require.Equal(t, uint64(30), aliceTxList.getTxByIndex(1).GetEstimatedConsumption())
	})

	t.Run("should sort same nonce and same estimated consumption by tx hash", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()

		err := mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash2"),
		})
		require.NoError(t, err)

		err = mempool.AddTransaction(&transaction{
			nonce:                1,
			sender:               []byte("alice"),
			estimatedConsumption: 10,
			txHash:               []byte("txHash1"),
		})
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, []byte("txHash1"), aliceTxList.getTxByIndex(0).GetTxHash())
		require.Equal(t, []byte("txHash2"), aliceTxList.getTxByIndex(1).GetTxHash())
	})
}
