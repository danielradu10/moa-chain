package localchain_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/explorer"
	"moa-chain/explorer/controllers"
	"moa-chain/explorer/service"
	"moa-chain/localchain"
)

// TestLocalChainIntegration starts a 5-node local chain, submits a transaction
// via POST, and verifies the full lifecycle (SUBMITTED → FINALIZED) through the
// SSE stream and snapshot endpoints.
func TestLocalChainIntegration(t *testing.T) {
	t.Parallel()

	lc, err := localchain.New(localchain.Config{
		NumNodes:   5,
		StartRound: 2,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	require.NoError(t, err)

	svc := service.NewExplorerService(lc.NodeView)
	handler := controllers.NewServer(svc, "").Handler()

	lc.Start()
	t.Cleanup(lc.Stop)

	// --- Submit two transactions via POST ---

	submitTx := func(body string) string {
		req := httptest.NewRequest("POST", "/api/v1/transactions", strings.NewReader(body))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusCreated, rec.Code)
		var resp explorer.SubmitTransactionResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.NotEmpty(t, resp.TxHash)
		return resp.TxHash
	}

	tx1Hash := submitTx(`{"sender":"alice","prompt":"Explain goroutines in Go.","nonce":0,"tip":10}`)
	tx2Hash := submitTx(`{"sender":"bob","prompt":"What is a mutex?","nonce":0,"tip":5}`)

	// --- SSE: subscribe to both and wait for both streams to close on FINALIZED ---
	// TxHub closes subscriber channels on FINALIZED, which unblocks the handler.

	sseCtx, cancelSSE := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelSSE()

	waitFinalized := func(hash string) chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			req := httptest.NewRequest("GET", "/api/v1/transactions/"+hash+"/events", nil).
				WithContext(sseCtx)
			handler.ServeHTTP(httptest.NewRecorder(), req)
		}()
		return done
	}

	tx1Done := waitFinalized(tx1Hash)
	tx2Done := waitFinalized(tx2Hash)

	for _, done := range []chan struct{}{tx1Done, tx2Done} {
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Fatal("tx SSE stream did not close after finalization")
		}
	}

	// --- Snapshot endpoint assertions (all post-finalization) ---

	// Health.
	{
		req := httptest.NewRequest("GET", "/api/v1/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"ok"`)
	}

	// Transaction lookup: both FINALIZED.
	for _, hash := range []string{tx1Hash, tx2Hash} {
		req := httptest.NewRequest("GET", "/api/v1/transactions/"+hash, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), "FINALIZED")
	}

	// Transactions list: both finalized txs appear, confirming ResolveAll walks
	// all chain blocks and not just the most recent one.
	{
		req := httptest.NewRequest("GET", "/api/v1/transactions", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		require.Contains(t, body, `"sender":"alice"`)
		require.Contains(t, body, `"sender":"bob"`)
	}

	// Round 2: the first auto-starting round must be finalized by the time any
	// tx is finalized (rounds are sequential).
	{
		req := httptest.NewRequest("GET", "/api/v1/rounds/2", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Contains(t, rec.Body.String(), `"status":"FINALIZED"`)
	}

	// Unknown round: not found.
	{
		req := httptest.NewRequest("GET", "/api/v1/rounds/9999", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNotFound, rec.Code)
	}

	// POST validation: empty sender → 400.
	{
		req := httptest.NewRequest("POST", "/api/v1/transactions",
			strings.NewReader(`{"prompt":"test"}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		require.Equal(t, http.StatusBadRequest, rec.Code)
	}
}

func TestLocalChainValidatorIDsValidation(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	_, err := localchain.New(localchain.Config{
		NumNodes:     2,
		ValidatorIDs: []string{"model-a"},
		Logger:       logger,
	})
	require.ErrorContains(t, err, "validator ID count")

	_, err = localchain.New(localchain.Config{
		NumNodes:     2,
		ValidatorIDs: []string{"model-a", "model-a"},
		Logger:       logger,
	})
	require.ErrorContains(t, err, "duplicate validator ID")
}
