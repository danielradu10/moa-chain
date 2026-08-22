package resolvers_test

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/explorer/resolvers"
	"moa-chain/explorer/testscommon"
	moacommon "moa-chain/testscommon"
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

// --- TxResolver ---

func TestTxResolver_InvalidHex(t *testing.T) {
	t.Parallel()

	r := resolvers.NewTxResolver(&testscommon.NodeFacadeStub{})
	_, ok := r.Resolve("not-valid-hex!!")
	require.False(t, ok)
}

func TestTxResolver_NotFound(t *testing.T) {
	t.Parallel()

	r := resolvers.NewTxResolver(&testscommon.NodeFacadeStub{})
	_, ok := r.Resolve(hex.EncodeToString([]byte("unknown")))
	require.False(t, ok)
}

func TestTxResolver_Submitted(t *testing.T) {
	t.Parallel()

	hashBytes := []byte("tx-submitted")
	stub := &testscommon.NodeFacadeStub{
		TxStatuses: map[string]tracker.TxStatus{
			string(hashBytes): tracker.TxStatusSubmitted,
		},
	}

	r := resolvers.NewTxResolver(stub)
	resp, ok := r.Resolve(hex.EncodeToString(hashBytes))

	require.True(t, ok)
	require.Equal(t, "SUBMITTED", resp.Status)
	require.Equal(t, hex.EncodeToString(hashBytes), resp.TxHash)
	require.Empty(t, resp.BlockHash)
	require.Empty(t, resp.FinalAnswer)
}

func TestTxResolver_Pending_EnrichesFromMempoolAndStore(t *testing.T) {
	t.Parallel()

	hashBytes := []byte("tx-pending")
	tx := &moacommon.TransactionStub{}
	tx.SetTxHash(hashBytes)
	tx.SetSender([]byte("alice"))
	tx.SetPrompt([]byte("what is 2+2?"))

	stub := &testscommon.NodeFacadeStub{
		TxStatuses: map[string]tracker.TxStatus{
			string(hashBytes): tracker.TxStatusPending,
		},
		PendingTransactions: []data.Transaction{tx},
		PrecomputedLabels:   map[string][]string{string(hashBytes): {"math"}},
		PrecomputedAnswers:  map[string]string{string(hashBytes): "4"},
	}

	r := resolvers.NewTxResolver(stub)
	resp, ok := r.Resolve(hex.EncodeToString(hashBytes))

	require.True(t, ok)
	require.Equal(t, "PENDING", resp.Status)
	require.Equal(t, "alice", resp.Sender)
	require.Equal(t, "what is 2+2?", resp.Prompt)
	require.Equal(t, []string{"math"}, resp.Labels)
	require.Equal(t, "4", resp.LocalAnswer)
}

func TestTxResolver_Finalized_EnrichesFromChain(t *testing.T) {
	t.Parallel()

	hashBytes := []byte("tx-finalized")
	blockHash := []byte("block-hash")

	tx := &moacommon.TransactionStub{}
	tx.SetTxHash(hashBytes)
	tx.SetSender([]byte("bob"))
	tx.SetPrompt([]byte("explain recursion"))

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: blockHash},
		Body:   data.BlockBody{Transactions: []data.Transaction{tx}},
		FinalAnswers: []data.FinalAnswer{
			{TxHash: hashBytes, Status: data.FinalAnswerStatusSynthesized, Answer: "recursion is..."},
		},
	}

	stub := &testscommon.NodeFacadeStub{
		TxStatuses: map[string]tracker.TxStatus{
			string(hashBytes): tracker.TxStatusFinalized,
		},
		Blocks: []*data.BlockOnChain{block},
	}

	r := resolvers.NewTxResolver(stub)
	resp, ok := r.Resolve(hex.EncodeToString(hashBytes))

	require.True(t, ok)
	require.Equal(t, "FINALIZED", resp.Status)
	require.Equal(t, "bob", resp.Sender)
	require.Equal(t, "explain recursion", resp.Prompt)
	require.Equal(t, hex.EncodeToString(blockHash), resp.BlockHash)
	require.Equal(t, "recursion is...", resp.FinalAnswer)
	require.Equal(t, "SYNTHESIZED", resp.FinalStatus)
}

// --- TxResolver.ResolveAll ---

func TestTxResolver_ResolveAll_Empty(t *testing.T) {
	t.Parallel()

	r := resolvers.NewTxResolver(&testscommon.NodeFacadeStub{})
	result := r.ResolveAll()
	require.Empty(t, result)
}

func TestTxResolver_ResolveAll_FinalizedThenPending(t *testing.T) {
	t.Parallel()

	finalizedHash := []byte("tx-final")
	pendingHash := []byte("tx-pending")

	finalTx := &moacommon.TransactionStub{}
	finalTx.SetTxHash(finalizedHash)
	finalTx.SetSender([]byte("alice"))

	pendingTx := &moacommon.TransactionStub{}
	pendingTx.SetTxHash(pendingHash)
	pendingTx.SetSender([]byte("bob"))

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("block-hash")},
		Body:   data.BlockBody{Transactions: []data.Transaction{finalTx}},
	}

	stub := &testscommon.NodeFacadeStub{
		Blocks: []*data.BlockOnChain{block},
		TxStatuses: map[string]tracker.TxStatus{
			string(pendingHash): tracker.TxStatusPending,
		},
		PendingTransactions: []data.Transaction{pendingTx},
	}

	r := resolvers.NewTxResolver(stub)
	result := r.ResolveAll()

	require.Len(t, result, 2)
	require.Equal(t, "FINALIZED", result[0].Status)
	require.Equal(t, "alice", result[0].Sender)
	require.Equal(t, "PENDING", result[1].Status)
	require.Equal(t, "bob", result[1].Sender)
}

func TestTxResolver_ResolveAll_NoDuplicates(t *testing.T) {
	t.Parallel()

	hashBytes := []byte("tx-dupe")

	tx := &moacommon.TransactionStub{}
	tx.SetTxHash(hashBytes)

	block := &data.BlockOnChain{
		Header: data.ChainBlockHeader{HeaderHash: []byte("block-hash")},
		Body:   data.BlockBody{Transactions: []data.Transaction{tx}},
	}

	stub := &testscommon.NodeFacadeStub{
		Blocks:              []*data.BlockOnChain{block},
		PendingTransactions: []data.Transaction{tx},
	}

	r := resolvers.NewTxResolver(stub)
	result := r.ResolveAll()

	require.Len(t, result, 1)
	require.Equal(t, "FINALIZED", result[0].Status)
}
