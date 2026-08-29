package integrationtests

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/tracker"
	"moa-chain/validators"
)

// TestExplorerIntegration starts a 5-node chain with a shared tx peer registry,
// wires an explorer server (with tx submission) to node 0, and verifies:
//   - All snapshot endpoints (health, block, round, transaction list/lookup)
//   - Live SSE endpoints (round step stream, tx lifecycle stream)
//   - POST /api/v1/transactions: submit a second tx via the HTTP endpoint and
//     track its full lifecycle (SUBMITTED → PREPROCESSING → PENDING → FINALIZED)
//     through the SSE stream.
func TestExplorerIntegration(t *testing.T) {
	t.Parallel()

	const numValidators = 5

	publicKeys, privateKeys := generateScenarioKeys(t, numValidators)
	registeredValidators := createScenarioValidators(publicKeys)
	inboxes := makeScenarioInboxes(numValidators)

	peersRegistry := broadcast.NewPeerRegistry()
	for i, v := range registeredValidators {
		require.NoError(t, peersRegistry.Register(v.PublicID(), inboxes[i]))
	}
	broadcaster := broadcast.NewBroadcaster(peersRegistry)
	batchAgent := continuousRoundAgent()

	// Shared tx registry: submitting on one node broadcasts the raw tx to all
	// others, so each node preprocesses independently. This is required for
	// POST-submitted transactions to reach validators other than node 0.
	txPeerRegistry := broadcast.NewTxPeerRegistry()

	roundHub := explorer.NewRoundHub()

	nodes := make([]*integrationTestNode, numValidators)
	for i, v := range registeredValidators {
		var onStepChanged func(data.RoundKey, data.Step)
		if i == 0 {
			onStepChanged = roundHub.OnStepChanged
		}
		nodes[i] = createNodeWithBroadcaster(
			t,
			v.PublicID(),
			privateKeys[i],
			registeredValidators,
			inboxes[i],
			nil, // no pre-loaded transactions — preprocessors stay alive during consensus
			batchAgent,
			broadcaster,
			false,
			false,
			0,
			validators.CommitteeStrategyHalf,
			0,
			txPeerRegistry, onStepChanged,
		)
	}

	// Wire TxHub to node 0's tracker so SSE subscribers get status events.
	txHub := explorer.NewTxHub()
	nodes[0].txTracker.OnStatusChanged = txHub.OnStatusChanged

	// Wire RoundTracker to node 0's blockFinalizer and mark txs FINALIZED when
	// MR3 completes. Must be called before any mini-rounds start.
	roundTracker := tracker.NewRoundTracker()
	node0TxTracker := nodes[0].txTracker
	nodes[0].blockFinalizer.WithHooks(blockFinalizer.BlockFinalizerHooks{
		OnMR1Finalized: roundTracker.OnMR1Finalized,
		OnMR2Finalized: roundTracker.OnMR2Finalized,
		OnMR3Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
			roundTracker.OnMR3Finalized(key, block)
			// MR3 block carries the MR1 Body.Transactions — mark each finalized.
			for _, tx := range block.Body.Transactions {
				node0TxTracker.OnFinalized(string(tx.GetTxHash()))
			}
		},
	})

	// Build the explorer server wired to node 0's state.
	// TxSubmitter enables POST /api/v1/transactions.
	nodeView := &explorer.NodeView{
		Chain:           nodes[0].chain,
		BlockchainState: nodes[0].blockchainState,
		BlockFinalizer:  nodes[0].blockFinalizer,
		Store:           nodes[0].store,
		Mempool:         nodes[0].mempool,
		TxTracker:       nodes[0].txTracker,
		RoundTracker:    roundTracker,
		RoundLoop:       nodes[0].loop,
		RoundHub:        roundHub,
		TxHub:           txHub,
		TxSubmitter:     nodes[0].txInterceptor,
	}
	svc := service.NewExplorerService(nodeView)
	server := controllers.NewServer(svc, "")
	handler := server.Handler()

	// --- tx1 ("alice"): submitted directly to node 0, broadcast to others ---

	tx1 := createTransaction("alice", 0, "Implement a concurrent hash map in Go.", 50, 1,
		[]string{"back_end_with_apis", "systems_programming"})
	tx1HexHash := hex.EncodeToString(tx1.GetTxHash())

	// Subscribe to SSE streams before submitting so early status events are
	// captured or replayed from hub.last.
	roundStreamCtx, cancelRoundStream := context.WithCancel(context.Background())
	defer cancelRoundStream()

	roundStreamRec := httptest.NewRecorder()
	roundStreamDone := make(chan struct{})
	go func() {
		defer close(roundStreamDone)
		req := httptest.NewRequest("GET", "/api/v1/round/stream", nil).WithContext(roundStreamCtx)
		handler.ServeHTTP(roundStreamRec, req)
	}()

	tx1StreamRec := httptest.NewRecorder()
	tx1StreamDone := make(chan struct{})
	go func() {
		defer close(tx1StreamDone)
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+tx1HexHash+"/events", nil)
		handler.ServeHTTP(tx1StreamRec, req)
	}()

	// Submit tx1 only to node 0; broadcast delivers it to every other node.
	require.NoError(t, nodes[0].txInterceptor.Submit(tx1))

	// --- tx2 ("bob"): submitted via POST /api/v1/transactions ---

	postBody := `{"sender":"bob","prompt":"Explain Go interfaces in depth.","nonce":0,"tip":10}`
	postReq := httptest.NewRequest("POST", "/api/v1/transactions", strings.NewReader(postBody))
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)
	require.Equal(t, http.StatusCreated, postRec.Code, "POST /api/v1/transactions should return 201")

	var postResp explorer.SubmitTransactionResponse
	require.NoError(t, json.NewDecoder(postRec.Body).Decode(&postResp))
	require.NotEmpty(t, postResp.TxHash, "POST response must include a tx hash")

	tx2HexHash := postResp.TxHash

	// Subscribe to tx2's SSE after POST (hub.last replays the most recent status).
	tx2StreamRec := httptest.NewRecorder()
	tx2StreamDone := make(chan struct{})
	go func() {
		defer close(tx2StreamDone)
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+tx2HexHash+"/events", nil)
		handler.ServeHTTP(tx2StreamRec, req)
	}()

	// Wait for tx1 to reach PENDING on ALL nodes (broadcast + preprocessing done).
	require.Eventually(t, func() bool {
		for _, node := range nodes {
			status, ok := node.txTracker.GetStatus(string(tx1.GetTxHash()))
			if !ok || status != tracker.TxStatusPending {
				return false
			}
		}
		return true
	}, 15*time.Second, 10*time.Millisecond, "tx1 should reach PENDING on all nodes")

	// Wait for tx2 to reach PENDING on node 0.
	// The raw hash is the hex-decoded response hash.
	tx2RawHashForWait, _ := hex.DecodeString(tx2HexHash)
	require.Eventually(t, func() bool {
		s, ok := nodes[0].txTracker.GetStatus(string(tx2RawHashForWait))
		return ok && s == tracker.TxStatusPending
	}, 15*time.Second, 10*time.Millisecond, "tx2 should reach PENDING on node 0")

	done := startScenarioNodes(nodes)
	stopNodes := stopScenarioNodesOnce(t, nodes, done)
	t.Cleanup(stopNodes)

	startScenarioRound(inboxes, data.RoundKey{
		Epoch:     0,
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	})

	// Wait for both transactions to appear in finalized blocks on every node.
	// tx2 may land in round 2 or a subsequent auto-advanced round.
	waitForAllNodesToContainTx(t, nodes, tx1.GetTxHash(), 30*time.Second)
	waitForAllNodesToContainTx(t, nodes, tx2RawHashForWait, 30*time.Second)

	// Both tx SSE streams close naturally when FINALIZED is emitted.
	select {
	case <-tx1StreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("tx1 SSE stream did not close after finalization")
	}
	select {
	case <-tx2StreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("tx2 SSE stream did not close after finalization")
	}

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	// Round stream does not close automatically; cancel context to unblock handler.
	cancelRoundStream()
	select {
	case <-roundStreamDone:
	case <-time.After(5 * time.Second):
		t.Fatal("round SSE stream did not return after context cancel")
	}

	// --- Snapshot endpoint assertions ---

	// Health.
	{
		req := httptest.NewRequest("GET", "/api/v1/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"ok"`)
	}

	// Block lookup: find the block containing tx1.
	var tx1Block *data.BlockOnChain
	for _, block := range nodes[0].chain.Blocks() {
		for _, bTx := range block.Body.Transactions {
			if string(bTx.GetTxHash()) == string(tx1.GetTxHash()) {
				tx1Block = block
			}
		}
	}
	require.NotNil(t, tx1Block, "tx1 should be in a finalized block")
	{
		blockHashHex := hex.EncodeToString(tx1Block.Header.HeaderHash)
		req := httptest.NewRequest("GET", "/api/v1/blocks/"+blockHashHex, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), tx1HexHash)
	}

	// Round 2: fully finalized with MR1 + MR2 + MR3.
	{
		req := httptest.NewRequest("GET", "/api/v1/rounds/2", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `"status":"FINALIZED"`)
		require.Contains(t, body, `"mr3"`)
	}

	// Round 99: not found.
	{
		req := httptest.NewRequest("GET", "/api/v1/rounds/99", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// Transactions list: both txs are present.
	{
		req := httptest.NewRequest("GET", "/api/v1/transactions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, tx1HexHash)
		require.Contains(t, body, tx2HexHash)
	}

	// tx1 FINALIZED via GET.
	{
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+tx1HexHash, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "FINALIZED")
	}

	// tx2 FINALIZED via GET.
	{
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+tx2HexHash, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "FINALIZED")
	}

	// Unknown tx: not found.
	{
		unknownHash := hex.EncodeToString(make([]byte, 32))
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+unknownHash, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// Round current: round loop is wired and returns a step.
	{
		req := httptest.NewRequest("GET", "/api/v1/round/current", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"step"`)
	}

	// POST with missing fields: bad request.
	{
		req := httptest.NewRequest("POST", "/api/v1/transactions", strings.NewReader(`{"sender":""}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}

	// --- Live SSE assertions ---

	// Round stream: multiple step-change events.
	roundStreamBody := roundStreamRec.Body.String()
	require.Greater(t, strings.Count(roundStreamBody, "data:"), 1,
		"round stream should have received multiple step events, got:\n%s", roundStreamBody)

	// tx1 stream: PENDING replayed or captured + FINALIZED.
	tx1StreamBody := tx1StreamRec.Body.String()
	require.Contains(t, tx1StreamBody, "PENDING", "tx1 SSE stream should contain PENDING")
	require.Contains(t, tx1StreamBody, "FINALIZED", "tx1 SSE stream should contain FINALIZED")

	// tx2 stream (POST-submitted): tracks full lifecycle ending at FINALIZED.
	tx2StreamBody := tx2StreamRec.Body.String()
	require.Contains(t, tx2StreamBody, "FINALIZED", "tx2 SSE stream should contain FINALIZED")
}
