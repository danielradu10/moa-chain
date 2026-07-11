//go:build integration

package integrationtests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"

	"moa-chain/agent"
	"moa-chain/data"
)

// benchmarkLabelingResult is one data point written to benchmark_labeling_results.jsonl.
type benchmarkLabelingResult struct {
	Experiment       string   `json:"experiment"`
	Run              int      `json:"run"`
	NumTxs           int      `json:"num_txs"`
	TotalInputTokens uint64   `json:"total_input_tokens"`
	AvgTokensPerTx   uint64   `json:"avg_tokens_per_tx"`
	DurationMs       int64    `json:"duration_ms"`
	LabelsProduced   []string `json:"labels_produced"`
}

// TestMiniRoundOne_LabelingLatencyBenchmark measures how LLM labeling latency
// scales with (a) the number of transactions per batch and (b) prompt length.
//
// Experiment A — fixed short prompt, vary tx count: 1, 2, 4, 8, 16
// Experiment B — fixed 4 txs, vary prompt length: short, medium, long, very_long
//
// Each combination is repeated 3 times to smooth out warm-model variance.
// Results are appended to testData/miniround1/results/benchmark_labeling_results.jsonl.
//
// Run with: make test-realagent-mr1-benchmark
func TestMiniRoundOne_LabelingLatencyBenchmark(t *testing.T) {
	pingAgentOrSkip(t)

	client := realAgentClient()

	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)

	countTokens := func(prompts []string) uint64 {
		var total uint64
		for _, p := range prompts {
			n, _ := enc.Count(p)
			total += uint64(n)
		}
		return total
	}

	resultsPath := filepath.Join(
		"testData", "miniround1", "results", "benchmark_labeling_results.jsonl",
	)
	f, err := os.OpenFile(resultsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	writeResult := func(r benchmarkLabelingResult) {
		b, _ := json.Marshal(r)
		_, _ = fmt.Fprintf(f, "%s\n", b)
	}

	flattenLabels := func(results []agent.LabelResult) []string {
		seen := make(map[string]struct{})
		var out []string
		for _, r := range results {
			for _, l := range r.Labels {
				if _, ok := seen[l]; !ok {
					seen[l] = struct{}{}
					out = append(out, l)
				}
			}
		}
		return out
	}

	// cooldown gives Ollama time to free resources between consecutive calls.
	// Without this, back-to-back long calls cause transient HTTP 400 errors.
	cooldown := func() { time.Sleep(3 * time.Second) }

	// ── Experiment A: vary tx count ────────────────────────────────────────────
	// A realistic short developer question, kept fixed so tx count is the only
	// variable. This isolates how latency scales with batch size.
	shortPrompt := "What is the difference between an array and a linked list in terms of memory layout and time complexity for insertion and access operations?"

	txCounts := []int{1, 2, 4, 8, 16}
	const runsPerPoint = 3

	for _, n := range txCounts {
		prompts := make([]string, n)
		for i := range prompts {
			prompts[i] = shortPrompt
		}
		totalTokens := countTokens(prompts)

		for run := 1; run <= runsPerPoint; run++ {
			cooldown()
			txs := makeBenchmarkTxs(prompts)
			t.Logf("[A] n=%d run=%d total_tokens=%d — calling LabelBatch...", n, run, totalTokens)

			t0 := time.Now()
			results, callErr := client.LabelBatch(txs)
			dur := time.Since(t0)
			if callErr != nil {
				t.Logf("[A] n=%d run=%d → SKIPPED (%v)", n, run, callErr)
				continue
			}

			labels := flattenLabels(results)
			t.Logf("[A] n=%d run=%d → %dms labels=%v", n, run, dur.Milliseconds(), labels)

			writeResult(benchmarkLabelingResult{
				Experiment:       "A_vary_tx_count",
				Run:              run,
				NumTxs:           n,
				TotalInputTokens: totalTokens,
				AvgTokensPerTx:   totalTokens / uint64(n),
				DurationMs:       dur.Milliseconds(),
				LabelsProduced:   labels,
			})
		}
	}

	// ── Experiment B: vary prompt length ───────────────────────────────────────
	// Fixed 4 txs with identical prompts at each length tier. This isolates how
	// latency scales with token count per transaction.
	promptTiers := []struct {
		tier   string
		prompt string
	}{
		{
			tier:   "short",
			prompt: "What is the difference between an array and a linked list?",
		},
		{
			tier:   "medium",
			prompt: "How does backpropagation work in a neural network and why is the chain rule needed to compute gradients through multiple layers of the computation graph?",
		},
		{
			tier: "long",
			prompt: "You are reviewing a pull request for a distributed key-value store written in Go. " +
				"The PR introduces a new replication protocol that uses a leader-follower model with " +
				"synchronous replication for writes and asynchronous replication for reads. The author " +
				"claims this improves read throughput at the cost of slightly higher write latency. " +
				"Describe the software engineering subdomain this work belongs to, any consistency " +
				"trade-offs introduced by mixing synchronous and asynchronous replication, and what " +
				"failure scenarios could violate the intended consistency model.",
		},
		{
			tier: "very_long",
			prompt: "We are building a machine learning pipeline that ingests raw sensor data from IoT " +
				"devices, preprocesses it using a streaming data platform, trains a time-series anomaly " +
				"detection model using an LSTM-based architecture, packages the trained model as a Docker " +
				"container, deploys it to a Kubernetes cluster with autoscaling enabled, exposes it as a " +
				"REST API behind an API gateway with rate limiting, and stores predictions in a time-series " +
				"database for downstream analytics dashboards. The pipeline must handle at least 100,000 " +
				"events per second with a processing latency of under 200ms, and the model must be " +
				"retrained automatically whenever drift is detected. " +
				"Identify all the software engineering subdomains involved in this system, explain how " +
				"each component maps to a subdomain, and describe the most critical integration points " +
				"where subdomain expertise overlap creates risk. Focus on the data flow from sensor to " +
				"dashboard and highlight where the ML subdomain intersects with the infrastructure and " +
				"cloud engineering subdomains.",
		},
	}

	const numTxsB = 4

	for _, pl := range promptTiers {
		prompts := make([]string, numTxsB)
		for i := range prompts {
			prompts[i] = pl.prompt
		}
		totalTokens := countTokens(prompts)

		for run := 1; run <= runsPerPoint; run++ {
			cooldown()
			txs := makeBenchmarkTxs(prompts)
			t.Logf("[B] tier=%s run=%d total_tokens=%d — calling LabelBatch...", pl.tier, run, totalTokens)

			t0 := time.Now()
			results, callErr := client.LabelBatch(txs)
			dur := time.Since(t0)
			if callErr != nil {
				t.Logf("[B] tier=%s run=%d → SKIPPED (%v)", pl.tier, run, callErr)
				continue
			}

			labels := flattenLabels(results)
			t.Logf("[B] tier=%s run=%d → %dms labels=%v", pl.tier, run, dur.Milliseconds(), labels)

			writeResult(benchmarkLabelingResult{
				Experiment:       "B_vary_prompt_length_" + pl.tier,
				Run:              run,
				NumTxs:           numTxsB,
				TotalInputTokens: totalTokens,
				AvgTokensPerTx:   totalTokens / numTxsB,
				DurationMs:       dur.Milliseconds(),
				LabelsProduced:   labels,
			})
		}
	}
}

func makeBenchmarkTxs(prompts []string) []data.Transaction {
	txs := make([]data.Transaction, len(prompts))
	for i, p := range prompts {
		txs[i] = createTransaction(
			fmt.Sprintf("benchsender-%d", i),
			0,
			p,
			10,
			uint64(900000+i),
			nil,
		)
	}
	return txs
}
