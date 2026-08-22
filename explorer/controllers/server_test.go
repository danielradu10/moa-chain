package controllers_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/mempool"
	"moa-chain/testscommon"
	"moa-chain/tracker"
	"moa-chain/txpipeline"
)

func newTestNodeView(t *testing.T) *explorer.NodeView {
	t.Helper()
	return &explorer.NodeView{
		Chain:             chain.NewChain(),
		BlockchainState:   &testscommon.BlockchainStateStub{},
		BlockFinalizer:    blockFinalizer.NewFinalizeBlockComponent(),
		ValidatorRegistry: &testscommon.ValidatorRegistryStub{},
		Store:             txpipeline.NewPrecomputedStore(),
		Mempool:           mempool.NewMemPool(),
		TxTracker:         tracker.NewTxTracker(),
		RoundTracker:      tracker.NewRoundTracker(),
	}
}

func newTestServer(t *testing.T, node *explorer.NodeView) *controllers.Server {
	t.Helper()
	return controllers.NewServer(service.NewExplorerService(node), "")
}

func doRequest(t *testing.T, s *controllers.Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func TestServer_Health(t *testing.T) {
	t.Parallel()

	t.Run("returns 200 with zero values on empty node", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))

		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var resp explorer.HealthResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, "ok", resp.Status)
		require.Equal(t, uint64(0), resp.ChainLength)
		require.Equal(t, uint64(0), resp.CurrentRound)
		require.Equal(t, uint64(0), resp.CurrentMiniRound)
		require.Equal(t, uint64(0), resp.CurrentEpoch)
	})

	t.Run("reflects blockchain state values", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		node.BlockchainState = &testscommon.BlockchainStateStub{
			CurrentRoundValue:     7,
			CurrentMiniRoundValue: 2,
			CurrentEpochValue:     1,
		}

		s := newTestServer(t, node)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.HealthResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, uint64(7), resp.CurrentRound)
		require.Equal(t, uint64(2), resp.CurrentMiniRound)
		require.Equal(t, uint64(1), resp.CurrentEpoch)
	})
}

func TestServer_Block(t *testing.T) {
	t.Parallel()

	t.Run("404 on empty chain", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/blocks/deadbeef")

		require.Equal(t, http.StatusNotFound, rec.Code)

		var errResp explorer.ErrorResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
		require.NotEmpty(t, errResp.Error)
	})

	t.Run("200 with block data when hash matches", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{
				HeaderHash: []byte("test-block-hash"),
				Round:      2,
				Epoch:      0,
			},
		}
		require.NoError(t, node.Chain.Append(block))

		s := newTestServer(t, node)
		hexHash := hex.EncodeToString([]byte("test-block-hash"))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/blocks/"+hexHash)

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.BlockResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, hexHash, resp.HeaderHash)
		require.Equal(t, uint64(2), resp.Round)
	})
}

func TestServer_Round(t *testing.T) {
	t.Parallel()

	t.Run("400 on non-numeric round", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/rounds/abc")

		require.Equal(t, http.StatusBadRequest, rec.Code)

		var errResp explorer.ErrorResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
		require.NotEmpty(t, errResp.Error)
	})

	t.Run("404 when round not in chain or tracker", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/rounds/42")

		require.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("200 FINALIZED from chain block", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{
				HeaderHash: []byte("round-block-hash"),
				Round:      5,
				Epoch:      0,
			},
		}
		require.NoError(t, node.Chain.Append(block))

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/rounds/5")

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.RoundResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, uint64(5), resp.Round)
		require.Equal(t, "FINALIZED", resp.Status)
		require.NotNil(t, resp.MR1)
	})

	t.Run("200 in-progress status from round tracker", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		mr1Block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{HeaderHash: []byte("mr1-hash")},
		}
		node.RoundTracker.OnMR1Finalized(data.RoundKey{}, mr1Block)

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/rounds/0")

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.RoundResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, string(tracker.RoundStatusMR1Complete), resp.Status)
		require.NotNil(t, resp.MR1)
		require.Nil(t, resp.MR2)
	})
}

func TestServer_RoundCurrent(t *testing.T) {
	t.Parallel()

	t.Run("503 when round loop not wired", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/round/current")

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("200 with step from round loop", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		node.RoundLoop = &liveStateReaderStub{
			key:  data.RoundKey{Epoch: 1, Round: 3, MiniRound: 0},
			step: data.StepCollectVotes,
		}

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/round/current")

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.LiveRoundResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, uint64(1), resp.Epoch)
		require.Equal(t, uint64(3), resp.Round)
		require.Equal(t, "COLLECT_VOTES", resp.Step)
	})
}

func TestServer_RoundStream(t *testing.T) {
	t.Parallel()

	t.Run("503 when hub not wired", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/round/stream")

		require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	})

	t.Run("streams step event as SSE", func(t *testing.T) {
		t.Parallel()

		node := newTestNodeView(t)
		hub := explorer.NewRoundHub()
		node.RoundHub = hub
		// Publish state before subscribing; new subscribers receive it immediately.
		hub.OnStepChanged(data.RoundKey{Epoch: 1, Round: 2}, data.StepCollectVotes)

		s := newTestServer(t, node)

		// 50ms is enough for the handler to subscribe, drain the buffered event,
		// and write it before the context cancels.
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/round/stream", nil)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
		require.Contains(t, rec.Body.String(), "COLLECT_VOTES")
	})
}

type liveStateReaderStub struct {
	key  data.RoundKey
	step data.Step
}

func (s *liveStateReaderStub) LiveState() (data.RoundKey, data.Step) {
	return s.key, s.step
}

func TestServer_Transactions(t *testing.T) {
	t.Parallel()

	t.Run("200 with empty list on empty node", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/transactions")

		require.Equal(t, http.StatusOK, rec.Code)

		var resp []explorer.TransactionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Empty(t, resp)
	})

	t.Run("200 includes finalized transactions from chain", func(t *testing.T) {
		t.Parallel()

		hashBytes := []byte("tx-list")
		tx := &testscommon.TransactionStub{}
		tx.SetTxHash(hashBytes)
		tx.SetSender([]byte("alice"))

		block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{HeaderHash: []byte("block-hash")},
			Body:   data.BlockBody{Transactions: []data.Transaction{tx}},
		}

		node := newTestNodeView(t)
		require.NoError(t, node.Chain.Append(block))

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/transactions")

		require.Equal(t, http.StatusOK, rec.Code)

		var resp []explorer.TransactionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Len(t, resp, 1)
		require.Equal(t, "FINALIZED", resp[0].Status)
		require.Equal(t, "alice", resp[0].Sender)
	})
}

func TestServer_Transaction(t *testing.T) {
	t.Parallel()

	t.Run("404 when tx not tracked", func(t *testing.T) {
		t.Parallel()

		s := newTestServer(t, newTestNodeView(t))
		rec := doRequest(t, s, http.MethodGet, "/api/v1/transactions/"+hex.EncodeToString([]byte("unknown")))

		require.Equal(t, http.StatusNotFound, rec.Code)

		var errResp explorer.ErrorResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&errResp))
		require.NotEmpty(t, errResp.Error)
	})

	t.Run("200 with submitted status", func(t *testing.T) {
		t.Parallel()

		hashBytes := []byte("tx-submitted")
		tx := &testscommon.TransactionStub{}
		tx.SetTxHash(hashBytes)

		node := newTestNodeView(t)
		node.TxTracker.OnSubmitted(tx)

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/transactions/"+hex.EncodeToString(hashBytes))

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.TransactionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, "SUBMITTED", resp.Status)
		require.Equal(t, hex.EncodeToString(hashBytes), resp.TxHash)
	})

	t.Run("200 finalized with block data", func(t *testing.T) {
		t.Parallel()

		hashBytes := []byte("tx-finalized")
		blockHash := []byte("block-hash")

		tx := &testscommon.TransactionStub{}
		tx.SetTxHash(hashBytes)
		tx.SetSender([]byte("alice"))
		tx.SetPrompt([]byte("what is 1+1?"))

		block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{HeaderHash: blockHash},
			Body:   data.BlockBody{Transactions: []data.Transaction{tx}},
			FinalAnswers: []data.FinalAnswer{
				{TxHash: hashBytes, Status: data.FinalAnswerStatusSynthesized, Answer: "2"},
			},
		}

		node := newTestNodeView(t)
		node.TxTracker.OnFinalized(string(hashBytes))
		require.NoError(t, node.Chain.Append(block))

		s := newTestServer(t, node)
		rec := doRequest(t, s, http.MethodGet, "/api/v1/transactions/"+hex.EncodeToString(hashBytes))

		require.Equal(t, http.StatusOK, rec.Code)

		var resp explorer.TransactionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Equal(t, "FINALIZED", resp.Status)
		require.Equal(t, "alice", resp.Sender)
		require.Equal(t, hex.EncodeToString(blockHash), resp.BlockHash)
		require.Equal(t, "2", resp.FinalAnswer)
		require.Equal(t, "SYNTHESIZED", resp.FinalStatus)
	})
}
