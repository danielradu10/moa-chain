package tracker_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/tracker"
)

func roundKey(epoch, round uint64) data.RoundKey {
	return data.RoundKey{Epoch: epoch, Round: round}
}

func TestRoundTracker_UnknownRound(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	_, ok := tr.GetRound(0, 1)
	require.False(t, ok)
}

func TestRoundTracker_MR1Only(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	block := &data.BlockOnChain{}

	tr.OnMR1Finalized(roundKey(0, 1), block)

	entry, ok := tr.GetRound(0, 1)
	require.True(t, ok)
	require.Equal(t, tracker.RoundStatusMR1Complete, entry.Status)
	require.Equal(t, block, entry.MR1Block)
	require.Nil(t, entry.MR2Block)
	require.Nil(t, entry.MR3Block)
}

func TestRoundTracker_ProgressionMR1ToMR3(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	mr1 := &data.BlockOnChain{}
	mr2 := &data.BlockOnChain{}
	mr3 := &data.BlockOnChain{}

	tr.OnMR1Finalized(roundKey(0, 2), mr1)
	entry, _ := tr.GetRound(0, 2)
	require.Equal(t, tracker.RoundStatusMR1Complete, entry.Status)

	tr.OnMR2Finalized(roundKey(0, 2), mr2)
	entry, _ = tr.GetRound(0, 2)
	require.Equal(t, tracker.RoundStatusMR2Complete, entry.Status)
	require.Equal(t, mr1, entry.MR1Block)
	require.Equal(t, mr2, entry.MR2Block)

	tr.OnMR3Finalized(roundKey(0, 2), mr3)
	entry, _ = tr.GetRound(0, 2)
	require.Equal(t, tracker.RoundStatusMR3Complete, entry.Status)
	require.Equal(t, mr3, entry.MR3Block)
}

func TestRoundTracker_MultipleRoundsIsolated(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	tr.OnMR1Finalized(roundKey(0, 1), &data.BlockOnChain{})
	tr.OnMR3Finalized(roundKey(0, 2), &data.BlockOnChain{})

	e1, ok1 := tr.GetRound(0, 1)
	e2, ok2 := tr.GetRound(0, 2)

	require.True(t, ok1)
	require.True(t, ok2)
	require.Equal(t, tracker.RoundStatusMR1Complete, e1.Status)
	require.Equal(t, tracker.RoundStatusMR3Complete, e2.Status)
	require.Nil(t, e1.MR2Block)
	require.Nil(t, e1.MR3Block)
}

func TestRoundTracker_EpochsIsolated(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	tr.OnMR1Finalized(roundKey(0, 1), &data.BlockOnChain{})
	tr.OnMR2Finalized(roundKey(1, 1), &data.BlockOnChain{})

	e0, _ := tr.GetRound(0, 1)
	e1, _ := tr.GetRound(1, 1)

	require.Equal(t, tracker.RoundStatusMR1Complete, e0.Status)
	require.Equal(t, tracker.RoundStatusMR2Complete, e1.Status)
}

func TestRoundTracker_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	tr := tracker.NewRoundTracker()
	const n = 50

	var wg sync.WaitGroup
	wg.Add(n * 3)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			tr.OnMR1Finalized(roundKey(0, uint64(i)), &data.BlockOnChain{})
		}(i)
		go func(i int) {
			defer wg.Done()
			tr.OnMR2Finalized(roundKey(0, uint64(i)), &data.BlockOnChain{})
		}(i)
		go func(i int) {
			defer wg.Done()
			tr.OnMR3Finalized(roundKey(0, uint64(i)), &data.BlockOnChain{})
		}(i)
	}
	wg.Wait()
}
