package mempool

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSendersMap_add(t *testing.T) {
	t.Parallel()

	tx1 := createTx(0, 1, []byte("txHash1"))
	tx2 := createTx(1, 2, []byte("txHash2"))
	tx3 := createTx(3, 4, []byte("txHash3"))
	tx4 := createTx(4, 5, []byte("txHash4"))
	tx5 := createTx(5, 6, []byte("txHash5"))

	sm := newSendersMap()
	sm.add("alice", tx2)
	sm.add("alice", tx1)
	sm.add("bob", tx4)
	sm.add("bob", tx3)
	sm.add("carol", tx5)

	expectedAliceList := newTxList()
	expectedAliceList.add(tx1)
	expectedAliceList.add(tx2)

	expectedBobList := newTxList()
	expectedBobList.add(tx3)
	expectedBobList.add(tx4)

	expectedCarolList := newTxList()
	expectedCarolList.add(tx5)

	require.Equal(t, 3, len(sm.senders))
	require.Equal(t, expectedAliceList, sm.senders["alice"])
	require.Equal(t, expectedBobList, sm.senders["bob"])
	require.Equal(t, expectedCarolList, sm.senders["carol"])
}

func TestSendersMap_addShouldCreateNewListForNewSender(t *testing.T) {
	t.Parallel()

	sm := newSendersMap()
	tx := createTx(7, 11, []byte("txHash1"))

	sm.add("alice", tx)

	require.Len(t, sm.senders, 1)
	require.Contains(t, sm.senders, "alice")
	require.NotNil(t, sm.senders["alice"])
	require.Equal(t, []Transaction{tx}, sm.senders["alice"].transactionList)
}

func TestSendersMap_addShouldKeepTransactionsSortedForSameSender(t *testing.T) {
	t.Parallel()

	sm := newSendersMap()

	tx1 := createTx(2, 20, []byte("txHash3"))
	tx2 := createTx(1, 30, []byte("txHash2"))
	tx3 := createTx(1, 10, []byte("txHash1"))

	sm.add("alice", tx1)
	sm.add("alice", tx2)
	sm.add("alice", tx3)

	expectedList := newTxList()
	expectedList.add(tx3)
	expectedList.add(tx2)
	expectedList.add(tx1)

	require.Equal(t, expectedList, sm.senders["alice"])
}

func TestSendersMap_addShouldNotAffectOtherSenders(t *testing.T) {
	t.Parallel()

	sm := newSendersMap()

	aliceTx1 := createTx(2, 20, []byte("txHash2"))
	aliceTx2 := createTx(1, 10, []byte("txHash1"))
	bobTx1 := createTx(5, 50, []byte("txHash3"))

	sm.add("alice", aliceTx1)
	sm.add("bob", bobTx1)
	sm.add("alice", aliceTx2)

	expectedAliceList := newTxList()
	expectedAliceList.add(aliceTx2)
	expectedAliceList.add(aliceTx1)

	expectedBobList := newTxList()
	expectedBobList.add(bobTx1)

	require.Equal(t, expectedAliceList, sm.senders["alice"])
	require.Equal(t, expectedBobList, sm.senders["bob"])
}

func TestSendersMap_addShouldReuseExistingListForSender(t *testing.T) {
	t.Parallel()

	sm := newSendersMap()

	tx1 := createTx(1, 10, []byte("txHash1"))
	tx2 := createTx(2, 20, []byte("txHash2"))

	sm.add("alice", tx1)
	initialListPointer := sm.senders["alice"]

	sm.add("alice", tx2)

	require.Same(t, initialListPointer, sm.senders["alice"])
	require.Equal(t, []Transaction{tx1, tx2}, sm.senders["alice"].transactionList)
}
