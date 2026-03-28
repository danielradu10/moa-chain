package mempool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_add(t *testing.T) {
	t.Parallel()

	tl := newTxList()

	tx1 := createTx(0, 10, []byte("txHash1"))
	tx2 := createTx(1, 10, []byte("txHash2"))
	tx3 := createTx(2, 10, []byte("txHash3"))
	transactions := []Transaction{
		tx3, tx1, tx2,
	}

	for _, tx := range transactions {
		tl.add(tx)
	}

	expectedTxList := []Transaction{
		tx1, tx2, tx3,
	}
	require.Equal(t, expectedTxList, tl.transactionList)
}

func Test_addShouldOrderByEstimatedConsumptionWhenNonceIsEqual(t *testing.T) {
	t.Parallel()

	tl := newTxList()

	tx1 := createTx(7, 30, []byte("txHash1"))
	tx2 := createTx(7, 10, []byte("txHash2"))
	tx3 := createTx(7, 20, []byte("txHash3"))

	transactions := []Transaction{
		tx1, tx2, tx3,
	}

	for _, tx := range transactions {
		tl.add(tx)
	}

	expectedTxList := []Transaction{
		tx2, tx3, tx1,
	}
	require.Equal(t, expectedTxList, tl.transactionList)
}

func Test_addShouldOrderByTxHashWhenNonceAndEstimatedConsumptionAreEqual(t *testing.T) {
	t.Parallel()

	tl := newTxList()

	tx1 := createTx(5, 100, []byte("txHash3"))
	tx2 := createTx(5, 100, []byte("txHash1"))
	tx3 := createTx(5, 100, []byte("txHash2"))

	transactions := []Transaction{
		tx1, tx2, tx3,
	}

	for _, tx := range transactions {
		tl.add(tx)
	}

	expectedTxList := []Transaction{
		tx2, tx3, tx1,
	}
	require.Equal(t, expectedTxList, tl.transactionList)
}

func Test_addShouldUseAllCriteria(t *testing.T) {
	t.Parallel()

	tl := newTxList()

	tx1 := createTx(1, 50, []byte("txHash3"))
	tx2 := createTx(0, 999, []byte("txHash5"))
	tx3 := createTx(1, 10, []byte("txHash4"))
	tx4 := createTx(1, 10, []byte("txHash1"))
	tx5 := createTx(2, 1, []byte("txHash2"))

	transactions := []Transaction{
		tx1, tx2, tx3, tx4, tx5,
	}

	for _, tx := range transactions {
		tl.add(tx)
	}

	expectedTxList := []Transaction{
		tx2, // nonce 0
		tx4, // nonce 1, consumption 10, hash txHash1
		tx3, // nonce 1, consumption 10, hash txHash4
		tx1, // nonce 1, consumption 50
		tx5, // nonce 2
	}
	require.Equal(t, expectedTxList, tl.transactionList)
}

func Test_findInsertionPlaceNoLockShouldReturnBeginning(t *testing.T) {
	t.Parallel()

	tl := newTxList()
	tx2 := createTx(1, 10, []byte("txHash2"))
	tx3 := createTx(2, 10, []byte("txHash3"))

	tl.transactionList = []Transaction{tx2, tx3}

	txToInsert := createTx(0, 10, []byte("txHash1"))

	position := tl.findInsertionPlaceNoLock(txToInsert)

	require.Equal(t, uint64(0), position)
}

func Test_findInsertionPlaceNoLockShouldReturnMiddle(t *testing.T) {
	t.Parallel()

	tl := newTxList()
	tx1 := createTx(0, 10, []byte("txHash1"))
	tx3 := createTx(2, 10, []byte("txHash3"))

	tl.transactionList = []Transaction{tx1, tx3}

	txToInsert := createTx(1, 10, []byte("txHash2"))

	position := tl.findInsertionPlaceNoLock(txToInsert)

	require.Equal(t, uint64(1), position)
}

func Test_findInsertionPlaceNoLockShouldReturnEnd(t *testing.T) {
	t.Parallel()

	tl := newTxList()
	tx1 := createTx(0, 10, []byte("txHash1"))
	tx2 := createTx(1, 10, []byte("txHash2"))

	tl.transactionList = []Transaction{tx1, tx2}

	txToInsert := createTx(2, 10, []byte("txHash3"))

	position := tl.findInsertionPlaceNoLock(txToInsert)

	require.Equal(t, uint64(2), position)
}

func Test_shouldComeBefore(t *testing.T) {
	t.Parallel()

	t.Run("should order by nonce first", func(t *testing.T) {
		t.Parallel()

		tx1 := createTx(1, 100, []byte("zzz"))
		tx2 := createTx(2, 1, []byte("aaa"))

		require.True(t, shouldComeBefore(tx1, tx2))
		require.False(t, shouldComeBefore(tx2, tx1))
	})

	t.Run("should order by estimated consumption when nonce is equal", func(t *testing.T) {
		t.Parallel()

		tx1 := createTx(1, 10, []byte("zzz"))
		tx2 := createTx(1, 20, []byte("aaa"))

		require.True(t, shouldComeBefore(tx1, tx2))
		require.False(t, shouldComeBefore(tx2, tx1))
	})

	t.Run("should order by tx hash when nonce and estimated consumption are equal", func(t *testing.T) {
		t.Parallel()

		tx1 := createTx(1, 10, []byte("aaa"))
		tx2 := createTx(1, 10, []byte("bbb"))

		require.True(t, shouldComeBefore(tx1, tx2))
		require.False(t, shouldComeBefore(tx2, tx1))
	})
}

func createTx(nonce uint64, estimatedConsumption uint64, txHash []byte) *transaction {
	return &transaction{
		nonce:                nonce,
		estimatedConsumption: estimatedConsumption,
		txHash:               txHash,
	}
}
