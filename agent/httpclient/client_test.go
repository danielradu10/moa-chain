package httpclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/data"
	"moa-chain/testscommon"
)

// newTestTx creates a TransactionStub with the given hash, prompt, and domain labels.
func newTestTx(hash, prompt string, labels []string) *testscommon.TransactionStub {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(hash))
	tx.SetPrompt([]byte(prompt))
	tx.SetDomainLabels(labels)
	return tx
}

// newTestClient creates a Client pointing at the given test server.
func newTestClient(server *httptest.Server, labelVersion, labelHash, answerVersion, answerHash string) *httpclient.Client {
	return httpclient.New(httpclient.Config{
		BaseURL:             server.URL,
		TimeoutSeconds:      5,
		LabelPromptVersion:  labelVersion,
		LabelPromptHash:     labelHash,
		AnswerPromptVersion: answerVersion,
		AnswerPromptHash:    answerHash,
	})
}

func TestClient_LabelBatch(t *testing.T) {
	t.Parallel()

	t.Run("success — maps labels from all transactions", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/label", r.URL.Path)
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_version": "labeler_v1",
				"prompt_hash":    "hash_abc",
				"results": []map[string]any{
					{
						"tx_hash": "txHash1",
						"labels": []map[string]any{
							{"subdomain": "databases", "confidence": 0.95},
						},
					},
					{
						"tx_hash": "txHash2",
						"labels": []map[string]any{
							{"subdomain": "security", "confidence": 0.8},
							{"subdomain": "back_end_with_apis", "confidence": 0.6},
						},
					},
				},
			})
		}))
		defer server.Close()

		client := newTestClient(server, "labeler_v1", "hash_abc", "", "")
		tx1 := newTestTx("txHash1", "Design a database schema", nil)
		tx2 := newTestTx("txHash2", "Secure an API endpoint", nil)

		results, err := client.LabelBatch([]data.Transaction{tx1, tx2})

		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, []byte("txHash1"), results[0].TxHash)
		require.Equal(t, []string{"databases"}, results[0].Labels)
		require.Equal(t, []byte("txHash2"), results[1].TxHash)
		require.Equal(t, []string{"security", "back_end_with_apis"}, results[1].Labels)
	})

	t.Run("prompt version mismatch in response — returns ErrPromptVersionMismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_version": "labeler_v99",
				"prompt_hash":    "hash_abc",
				"results":        []any{},
			})
		}))
		defer server.Close()

		client := newTestClient(server, "labeler_v1", "hash_abc", "", "")

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrPromptVersionMismatch)
	})

	t.Run("prompt hash mismatch in response — returns ErrPromptHashMismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_version": "labeler_v1",
				"prompt_hash":    "wrong_hash",
				"results":        []any{},
			})
		}))
		defer server.Close()

		client := newTestClient(server, "labeler_v1", "expected_hash", "", "")

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrPromptHashMismatch)
	})

	t.Run("server returns PROMPT_VERSION_MISMATCH error code — returns ErrPromptVersionMismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "PROMPT_VERSION_MISMATCH",
				"detail":    "loaded version is labeler_v1, got labeler_v99",
			})
		}))
		defer server.Close()

		client := newTestClient(server, "labeler_v99", "", "", "")

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrPromptVersionMismatch)
	})

	t.Run("server returns COVERAGE_MISMATCH — returns ErrCoverageMismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "COVERAGE_MISMATCH",
				"detail":    "tx_hash missing from response",
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrCoverageMismatch)
	})

	t.Run("non-200 without error body — returns ErrHTTPError", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrHTTPError)
	})

	t.Run("HTTP timeout — returns ErrProviderTimeout", func(t *testing.T) {
		t.Parallel()

		// handlerDone is closed by t.Cleanup so the blocking handler goroutine
		// exits before server.Close() tries to drain active connections.
		handlerDone := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			<-handlerDone
		}))
		t.Cleanup(func() {
			close(handlerDone)
			server.Close()
		})

		client := httpclient.New(httpclient.Config{
			BaseURL:        server.URL,
			TimeoutSeconds: 1,
		})

		_, err := client.LabelBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrProviderTimeout)
	})
}

func TestClient_AnswerBatch(t *testing.T) {
	t.Parallel()

	t.Run("success — maps answers from all transactions", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/answer", r.URL.Path)

			// Verify subdomains are forwarded from the transaction's domain labels.
			var req map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			txs := req["transactions"].([]any)
			firstTx := txs[0].(map[string]any)
			subdomains := firstTx["subdomains"].([]any)
			require.Equal(t, "databases", subdomains[0])

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_version": "answerer_v1",
				"prompt_hash":    "hash_def",
				"results": []map[string]any{
					{"tx_hash": "txHash1", "answer": "Use PostgreSQL with indexes."},
					{"tx_hash": "txHash2", "answer": "Implement JWT validation."},
				},
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "answerer_v1", "hash_def")
		tx1 := newTestTx("txHash1", "Design a DB", []string{"databases"})
		tx2 := newTestTx("txHash2", "Secure an API", []string{"security"})

		results, err := client.AnswerBatch([]data.Transaction{tx1, tx2})

		require.NoError(t, err)
		require.Len(t, results, 2)
		require.Equal(t, []byte("txHash1"), results[0].TxHash)
		require.Equal(t, "Use PostgreSQL with indexes.", results[0].Answer)
		require.Equal(t, []byte("txHash2"), results[1].TxHash)
		require.Equal(t, "Implement JWT validation.", results[1].Answer)
	})

	t.Run("prompt hash mismatch — returns ErrPromptHashMismatch", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"prompt_version": "answerer_v1",
				"prompt_hash":    "wrong_hash",
				"results":        []any{},
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "answerer_v1", "expected_hash")

		_, err := client.AnswerBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrPromptHashMismatch)
	})

	t.Run("server returns EMPTY_ANSWER error code — returns ErrEmptyAnswer", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "EMPTY_ANSWER",
				"detail":    "answer is empty after trimming",
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.AnswerBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrEmptyAnswer)
	})

	t.Run("server returns non-200 — returns ErrHTTPError", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.AnswerBatch(nil)

		require.ErrorIs(t, err, httpclient.ErrHTTPError)
	})
}

func TestClient_JudgeTransactionAnswers(t *testing.T) {
	t.Parallel()

	t.Run("success — returns raw response string unchanged", func(t *testing.T) {
		t.Parallel()

		rawJSON := `[{"tx_hash":"0xabc","category":"Correct","reason":"sound solution"}]`

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/judge", r.URL.Path)

			var req map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
			require.Equal(t, "system prompt", req["system_prompt"])
			require.Equal(t, "user prompt", req["user_prompt"])

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"response": rawJSON})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		got, err := client.JudgeTransactionAnswers(agent.AnswerJudgeRequest{
			SystemPrompt: "system prompt",
			UserPrompt:   "user prompt",
		})

		require.NoError(t, err)
		require.Equal(t, rawJSON, got)
	})

	t.Run("server returns INVALID_REQUEST error — returns ErrInvalidRequest", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "INVALID_REQUEST",
				"detail":    "system_prompt is empty",
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.JudgeTransactionAnswers(agent.AnswerJudgeRequest{})

		require.ErrorIs(t, err, httpclient.ErrInvalidRequest)
	})

	t.Run("server returns PROVIDER_ERROR — returns ErrProviderError", func(t *testing.T) {
		t.Parallel()

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": "PROVIDER_ERROR",
				"detail":    "ollama returned 503",
			})
		}))
		defer server.Close()

		client := newTestClient(server, "", "", "", "")

		_, err := client.JudgeTransactionAnswers(agent.AnswerJudgeRequest{
			SystemPrompt: "s",
			UserPrompt:   "u",
		})

		require.ErrorIs(t, err, httpclient.ErrProviderError)
	})
}
