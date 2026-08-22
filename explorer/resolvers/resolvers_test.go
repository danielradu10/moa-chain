package resolvers_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/explorer/resolvers"
	"moa-chain/explorer/testscommon"
	"moa-chain/tracker"
)

func hexHash(b []byte) string { return hex.EncodeToString(b) }

// --- BlockResolver ---

func TestBlockResolver_NotFound(t *testing.T) {
	t.Parallel()

	r := resolvers.NewBlockResolver(&testscommon.NodeFacadeStub{})
	_, ok := r.ResolveHash("deadbeef")
	require.False(t, ok)
}

func TestBlockResolver_Found(t *testing.T) {
	t.Parallel()

	hash := []byte("block-hash")
	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: hash, Round: 4, Epoch: 1},
	}

	r := resolvers.NewBlockResolver(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := r.ResolveHash(hexHash(hash))

	require.True(t, ok)
	require.Equal(t, hexHash(hash), resp.HeaderHash)
	require.Equal(t, uint64(4), resp.Round)
	require.Equal(t, uint64(1), resp.Epoch)
}

func TestBlockResolver_WrongHash(t *testing.T) {
	t.Parallel()

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("real-hash")},
	}

	r := resolvers.NewBlockResolver(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	_, ok := r.ResolveHash(hexHash([]byte("other-hash")))
	require.False(t, ok)
}

func TestBlockResolver_MapsTransactions(t *testing.T) {
	t.Parallel()

	hash := []byte("hash")
	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: hash},
		FinalAnswers: []data.FinalAnswer{
			{TxHash: []byte("tx1"), Status: data.FinalAnswerStatusSynthesized, Answer: "42"},
		},
	}

	r := resolvers.NewBlockResolver(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := r.ResolveHash(hexHash(hash))

	require.True(t, ok)
	require.Len(t, resp.FinalAnswers, 1)
	require.Equal(t, "SYNTHESIZED", resp.FinalAnswers[0].Status)
	require.Equal(t, "42", resp.FinalAnswers[0].Answer)
}

// --- RoundResolver ---

func TestRoundResolver_NotFound(t *testing.T) {
	t.Parallel()

	r := resolvers.NewRoundResolver(&testscommon.NodeFacadeStub{})
	_, ok := r.Resolve(99)
	require.False(t, ok)
}

func TestRoundResolver_FinalizedFromChain(t *testing.T) {
	t.Parallel()

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("hash"), Round: 3, Epoch: 0},
	}

	r := resolvers.NewRoundResolver(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := r.Resolve(3)

	require.True(t, ok)
	require.Equal(t, uint64(3), resp.Round)
	require.Equal(t, "FINALIZED", resp.Status)
	require.NotNil(t, resp.MR1)
	require.Nil(t, resp.MR2)
	require.Nil(t, resp.MR3)
}

func TestRoundResolver_MR2AndMR3PresentWhenPopulated(t *testing.T) {
	t.Parallel()

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("hash"), Round: 1},
		AnswerClassifications: []data.TransactionAnswerClassification{
			{TxHash: []byte("tx1"), Status: data.TransactionAnswerStatusReadyForMiniRoundThree},
		},
		FinalAnswers: []data.FinalAnswer{
			{TxHash: []byte("tx1"), Status: data.FinalAnswerStatusSynthesized, Answer: "answer"},
		},
	}

	r := resolvers.NewRoundResolver(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := r.Resolve(1)

	require.True(t, ok)
	require.NotNil(t, resp.MR2)
	require.NotNil(t, resp.MR3)
	require.Equal(t, "SYNTHESIZED", resp.MR3.FinalAnswers[0].Status)
}

func TestRoundResolver_InProgressFromTracker(t *testing.T) {
	t.Parallel()

	mr1Block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("mr1-hash")},
	}
	entry := tracker.RoundEntry{
		Epoch:    0,
		Round:    7,
		Status:   tracker.RoundStatusMR1Complete,
		MR1Block: mr1Block,
	}

	r := resolvers.NewRoundResolver(&testscommon.NodeFacadeStub{
		RoundEntries: map[uint64]tracker.RoundEntry{7: entry},
	})
	resp, ok := r.Resolve(7)

	require.True(t, ok)
	require.Equal(t, uint64(7), resp.Round)
	require.Equal(t, string(tracker.RoundStatusMR1Complete), resp.Status)
	require.NotNil(t, resp.MR1)
	require.Nil(t, resp.MR2)
	require.Nil(t, resp.MR3)
}

func TestRoundResolver_ChainTakesPriorityOverTracker(t *testing.T) {
	t.Parallel()

	chainBlock := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("chain-hash"), Round: 3},
	}
	trackerEntry := tracker.RoundEntry{
		Round:    3,
		Status:   tracker.RoundStatusMR1Complete,
		MR1Block: &data.BlockOnChain{},
	}

	r := resolvers.NewRoundResolver(&testscommon.NodeFacadeStub{
		Blocks:       []*data.BlockOnChain{chainBlock},
		RoundEntries: map[uint64]tracker.RoundEntry{3: trackerEntry},
	})
	resp, ok := r.Resolve(3)

	require.True(t, ok)
	require.Equal(t, "FINALIZED", resp.Status)
}
