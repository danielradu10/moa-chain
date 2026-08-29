package tracker_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/testscommon"
	"moa-chain/tracker"
)

func newTrackedTx(hash string) *testscommon.TransactionStub {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(hash))
	return tx
}

func TestTxTracker_UnknownHash(t *testing.T) {
	t.Parallel()

	tr := tracker.NewTxTracker()
	_, ok := tr.GetStatus("unknown")
	require.False(t, ok)
}

func TestTxTracker_StatusTransitions(t *testing.T) {
	t.Parallel()

	tr := tracker.NewTxTracker()
	tx := newTrackedTx("tx1")

	tr.OnSubmitted(tx)
	status, ok := tr.GetStatus("tx1")
	require.True(t, ok)
	require.Equal(t, tracker.TxStatusSubmitted, status)

	tr.OnPreprocessing(tx)
	status, _ = tr.GetStatus("tx1")
	require.Equal(t, tracker.TxStatusPreprocessing, status)

	tr.OnPending(tx)
	status, _ = tr.GetStatus("tx1")
	require.Equal(t, tracker.TxStatusPending, status)

	tr.OnFinalized("tx1")
	status, _ = tr.GetStatus("tx1")
	require.Equal(t, tracker.TxStatusFinalized, status)
}

func TestTxTracker_MultipleTransactions(t *testing.T) {
	t.Parallel()

	tr := tracker.NewTxTracker()

	tr.OnSubmitted(newTrackedTx("tx-a"))
	tr.OnPending(newTrackedTx("tx-b"))

	statusA, okA := tr.GetStatus("tx-a")
	statusB, okB := tr.GetStatus("tx-b")

	require.True(t, okA)
	require.True(t, okB)
	require.Equal(t, tracker.TxStatusSubmitted, statusA)
	require.Equal(t, tracker.TxStatusPending, statusB)
}

func TestTxTracker_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	tr := tracker.NewTxTracker()
	const n = 100

	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			tx := newTrackedTx(string(rune('a' + i%26)))
			tr.OnSubmitted(tx)
			tr.OnPreprocessing(tx)
			tr.OnPending(tx)
		}(i)
	}
	wg.Wait()
}
