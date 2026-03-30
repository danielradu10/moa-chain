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

func TestMemPool_SelectTransactions(t *testing.T) {
	t.Parallel()

	t.Run("should return empty list when mempool is empty", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{})

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Empty(t, selectedTransactions)
	})

	t.Run("should select transactions in comparator order across senders", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
			"carol": {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 50, 30, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 80, 20, 10, []byte("txHash2"))
		tx3 := createSelectionTx(0, "carol", 60, 10, 10, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx2, tx3, tx1}, selectedTransactions)
	})

	t.Run("should use estimated consumption as tie breaker when scores are equal", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 30, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 100, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx2, tx1}, selectedTransactions)
	})

	t.Run("should use tx hash as tie breaker when score and consumption are equal", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 100, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx1, tx2}, selectedTransactions)
	})

	t.Run("should stop when next best transaction exceeds max block consumption", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 9000, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 90, 2000, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx1}, selectedTransactions)
	})

	t.Run("should skip sender when first transaction has initial nonce gap", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(2, "alice", 100, 10, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 50, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx2}, selectedTransactions)
	})

	t.Run("should skip transaction when balance is insufficient", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 50},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 60, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 50, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx2}, selectedTransactions)
	})

	t.Run("should select multiple valid consecutive transactions from same sender", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 20, []byte("txHash1"))
		tx2 := createSelectionTx(1, "alice", 90, 10, 20, []byte("txHash2"))
		tx3 := createSelectionTx(0, "bob", 80, 10, 20, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx1, tx2, tx3}, selectedTransactions)
	})

	t.Run("should skip second transaction from sender when accumulated transferred value exceeds balance", func(t *testing.T) {
		t.Parallel()

		mempool := NewMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 50},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 30, []byte("txHash1"))
		tx2 := createSelectionTx(1, "alice", 90, 10, 30, []byte("txHash2"))
		tx3 := createSelectionTx(0, "bob", 80, 10, 10, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState, isTransactionMoreValuable)

		require.Equal(t, []Transaction{tx1, tx3}, selectedTransactions)
	})
}
