package service_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/explorer/service"
	"moa-chain/explorer/testscommon"
	"moa-chain/tracker"
)

func hexHash(b []byte) string { return hex.EncodeToString(b) }

// --- GetBlock tests ---

func TestGetBlock_NotFound(t *testing.T) {
	t.Parallel()

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{})
	_, ok := svc.GetBlock("deadbeef")
	require.False(t, ok)
}

func TestGetBlock_Found(t *testing.T) {
	t.Parallel()

	hash := []byte("block-hash-1")
	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: hash, Round: 3, Epoch: 0},
	}

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := svc.GetBlock(hexHash(hash))

	require.True(t, ok)
	require.Equal(t, hexHash(hash), resp.HeaderHash)
	require.Equal(t, uint64(3), resp.Round)
	require.Equal(t, uint64(0), resp.Epoch)
}

func TestGetBlock_ReturnsTransactions(t *testing.T) {
	t.Parallel()

	hash := []byte("block-hash-2")
	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: hash},
		Body:   data.BlockBody{Transactions: []data.Transaction{}},
	}

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := svc.GetBlock(hexHash(hash))

	require.True(t, ok)
	require.Empty(t, resp.Transactions)
}

// --- GetRound tests ---

func TestGetRound_NotFound(t *testing.T) {
	t.Parallel()

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{})
	_, ok := svc.GetRound(99)
	require.False(t, ok)
}

func TestGetRound_FinalizedFromChain(t *testing.T) {
	t.Parallel()

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("chain-block-hash"), Round: 5, Epoch: 0},
	}

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := svc.GetRound(5)

	require.True(t, ok)
	require.Equal(t, uint64(5), resp.Round)
	require.Equal(t, "FINALIZED", resp.Status)
	require.NotNil(t, resp.MR1)
	require.Nil(t, resp.MR2)
	require.Nil(t, resp.MR3)
}

func TestGetRound_InProgressFromTracker(t *testing.T) {
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

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{
		RoundEntries: map[uint64]tracker.RoundEntry{7: entry},
	})
	resp, ok := svc.GetRound(7)

	require.True(t, ok)
	require.Equal(t, uint64(7), resp.Round)
	require.Equal(t, string(tracker.RoundStatusMR1Complete), resp.Status)
	require.NotNil(t, resp.MR1)
	require.Nil(t, resp.MR2)
	require.Nil(t, resp.MR3)
}

func TestGetRound_ChainTakesPriorityOverTracker(t *testing.T) {
	t.Parallel()

	chainBlock := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("chain-hash"), Round: 3},
	}
	trackerEntry := tracker.RoundEntry{
		Round:    3,
		Status:   tracker.RoundStatusMR1Complete,
		MR1Block: &data.BlockOnChain{},
	}

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{
		Blocks:       []*data.BlockOnChain{chainBlock},
		RoundEntries: map[uint64]tracker.RoundEntry{3: trackerEntry},
	})
	resp, ok := svc.GetRound(3)

	require.True(t, ok)
	require.Equal(t, "FINALIZED", resp.Status)
}

func TestGetRound_MR2AndMR3PopulatedFromChain(t *testing.T) {
	t.Parallel()

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("hash"), Round: 1},
		AnswerClassifications: []data.TransactionAnswerClassification{
			{TxHash: []byte("tx1"), Status: data.TransactionAnswerStatusReadyForMiniRoundThree},
		},
		FinalAnswers: []data.FinalAnswer{
			{TxHash: []byte("tx1"), Status: data.FinalAnswerStatusSynthesized, Answer: "42"},
		},
	}

	svc := service.NewExplorerService(&testscommon.NodeFacadeStub{Blocks: []*data.BlockOnChain{block}})
	resp, ok := svc.GetRound(1)

	require.True(t, ok)
	require.NotNil(t, resp.MR2)
	require.NotNil(t, resp.MR3)
	require.Equal(t, "SYNTHESIZED", resp.MR3.FinalAnswers[0].Status)
	require.Equal(t, "42", resp.MR3.FinalAnswers[0].Answer)
}
