package controllers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/explorer"
	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/mempool"
	"moa-chain/testscommon"
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
	}
}

func newTestServer(t *testing.T, node *explorer.NodeView) *controllers.Server {
	t.Helper()
	return controllers.NewServer(service.NewExplorerService(node), "")
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
