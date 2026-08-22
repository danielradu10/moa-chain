package controllers_test

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
