//go:build integration

package integrationtests

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
)

// timingAgent wraps a BatchAgent and logs per-call latency and label results
// to both the test terminal (t.Logf) and a persistent timing.log file.
type timingAgent struct {
	t      *testing.T
	inner  agent.BatchAgent
	logger *slog.Logger
}

var _ agent.BatchAgent = (*timingAgent)(nil)

func (a *timingAgent) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	t0 := time.Now()
	result, err := a.inner.LabelBatch(txs)
	elapsed := time.Since(t0).Round(time.Millisecond)
	if err == nil {
		for _, r := range result {
			a.t.Logf("[labels] tx=%.8x → %v", r.TxHash, r.Labels)
			a.logger.Info("label_result", "tx", fmt.Sprintf("%.8x", r.TxHash), "labels", r.Labels)
		}
	}
	a.t.Logf("[timing] LabelBatch %d txs → %s (err=%v)", len(txs), elapsed, err)
	a.logger.Info("label_batch_timing", "txs", len(txs), "elapsed", elapsed, "err", err)
	return result, err
}

func (a *timingAgent) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	t0 := time.Now()
	result, err := a.inner.AnswerBatch(txs)
	elapsed := time.Since(t0).Round(time.Millisecond)
	a.t.Logf("[timing] AnswerBatch %d txs → %s (err=%v)", len(txs), elapsed, err)
	a.logger.Info("answer_batch_timing", "txs", len(txs), "elapsed", elapsed, "err", err)
	return result, err
}

func (a *timingAgent) JudgeTransactionAnswers(req agent.AnswerJudgeRequest) (string, error) {
	t0 := time.Now()
	result, err := a.inner.JudgeTransactionAnswers(req)
	elapsed := time.Since(t0).Round(time.Millisecond)
	a.t.Logf("[timing] JudgeTransactionAnswers → %s (err=%v)", elapsed, err)
	a.logger.Info("judge_timing", "elapsed", elapsed, "err", err)
	return result, err
}

// createTimingLogger opens (or creates) an append-mode log file at
// logs/<testName>/timing.log and returns a file-only slog.Logger and a close func.
func createTimingLogger(t *testing.T) (*slog.Logger, func()) {
	t.Helper()

	testName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	logPath := filepath.Join("logs", testName, "timing.log")

	err := os.MkdirAll(filepath.Dir(logPath), 0o755)
	require.NoError(t, err)

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)

	logger := slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return logger, func() { _ = f.Close() }
}

// repeatedRunResult is one data point saved after each run for later analysis
// (dominant labels, frequency variance, convergence rate, latency percentiles).
type repeatedRunResult struct {
	TestGroup             string                   `json:"testGroup"`
	RunIndex              int                      `json:"runIndex"`
	RoundNumber           uint64                   `json:"roundNumber"`
	Timestamp             string                   `json:"timestamp"`
	RoundDurationMs       int64                    `json:"roundDurationMs"`
	NumValidators         int                      `json:"numValidators"`
	NumTransactions       int                      `json:"numTransactions"`
	SubdomainsFrequencies data.SubdomainsFrequency `json:"subdomainsFrequencies"`
	NonRelatedTxHashes    []string                 `json:"nonRelatedTxHashes"`
}

// repeatedRunCount reads MOA_REPEATED_RUNS from the environment, defaulting to 5.
func repeatedRunCount() int {
	if v := os.Getenv("MOA_REPEATED_RUNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 5
}

// runRepeatedMR1Rounds runs mini-round one N times with the given transactions,
// saves each result to disk, and returns all results for in-test assertions.
// baseRound is the starting round number; each run increments it by one so that
// per-run block finalizers (in-memory, per-node) never see duplicate round keys.
func runRepeatedMR1Rounds(
	t *testing.T,
	testGroup string,
	transactions []data.Transaction,
	baseRound uint64,
) []repeatedRunResult {
	t.Helper()

	n := repeatedRunCount()
	results := make([]repeatedRunResult, 0, n)

	timingLog, closeLog := createTimingLogger(t)
	defer closeLog()

	wrapped := &timingAgent{t: t, inner: realAgentClient(), logger: timingLog}

	for i := 0; i < n; i++ {
		roundNumber := baseRound + uint64(i)

		t.Logf("=== repeated run %d/%d  group=%s  round=%d ===", i+1, n, testGroup, roundNumber)
		timingLog.Info("run_start", "run", i+1, "total", n, "group", testGroup, "round", roundNumber)

		runStart := time.Now()
		block := runRealAgentMR1Round(t, wrapped, transactions, roundNumber)
		duration := time.Since(runStart)

		result := repeatedRunResult{
			TestGroup:             testGroup,
			RunIndex:              i + 1,
			RoundNumber:           roundNumber,
			Timestamp:             time.Now().UTC().Format(time.RFC3339Nano),
			RoundDurationMs:       duration.Milliseconds(),
			NumValidators:         20,
			NumTransactions:       len(transactions),
			SubdomainsFrequencies: block.SubdomainsFrequencies,
			NonRelatedTxHashes:    block.NonRelatedTransactionHashes,
		}
		results = append(results, result)
		appendRepeatedRunResult(t, result)

		t.Logf("=== run %d/%d done — duration=%s frequencies=%v ===",
			i+1, n, duration.Round(time.Millisecond), block.SubdomainsFrequencies)
		timingLog.Info("run_end",
			"run", i+1,
			"duration", duration.Round(time.Millisecond),
			"frequencies", block.SubdomainsFrequencies,
			"non_related_count", len(block.NonRelatedTransactionHashes),
		)
	}

	return results
}

func appendRepeatedRunResult(t *testing.T, result repeatedRunResult) {
	t.Helper()

	encoded, err := json.Marshal(result)
	require.NoError(t, err)

	outputPath := filepath.Join("testData", "miniround1", "results", "repeated_runs_results.jsonl")
	err = os.MkdirAll(filepath.Dir(outputPath), 0o755)
	require.NoError(t, err)

	f, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, f.Close())
	}()

	_, err = f.Write(append(encoded, '\n'))
	require.NoError(t, err)
}

// TestMiniRoundOne_RealAgent_RepeatedRuns_GroupA runs the non-coding transaction
// set N times and asserts that every run converges to non_related. Results are
// saved to testData/miniround1/results/repeated_runs_results.jsonl for analysis.
//
// Run with: make test-realagent-mr1-repeated-a  (or MOA_REPEATED_RUNS=10 go test ...)
func TestMiniRoundOne_RealAgent_RepeatedRuns_GroupA_NonCodingConvergeToNonRelated(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createNonCodingTransactions()
	results := runRepeatedMR1Rounds(t, "GroupA_NonCoding", transactions, 100)

	for _, r := range results {
		require.Emptyf(t, r.SubdomainsFrequencies,
			"run %d: non-coding transactions must not produce subdomain frequencies; got %v",
			r.RunIndex, r.SubdomainsFrequencies)
		require.Lenf(t, r.NonRelatedTxHashes, len(transactions),
			"run %d: all %d transactions must be classified as non_related",
			r.RunIndex, len(transactions))
	}
}

// TestMiniRoundOne_RealAgent_RepeatedRuns_GroupB runs the clear-domain transaction
// set N times and asserts that every run produces the expected domain labels.
// Results are saved to testData/miniround1/results/repeated_runs_results.jsonl.
//
// Run with: make test-realagent-mr1-repeated-b  (or MOA_REPEATED_RUNS=10 go test ...)
func TestMiniRoundOne_RealAgent_RepeatedRuns_GroupB_ClearDomainConvergeToExpectedLabels(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createClearDomainTransactions()
	results := runRepeatedMR1Rounds(t, "GroupB_ClearDomain", transactions, 200)

	mustAppear := []string{
		"blockchain_engineering",
		"ml_ai_engineering",
		"cloud_engineering",
		"web_front_end",
	}
	mustNotAppear := []string{"mobile_dev", "data_engineering", "test_engineering_and_qa_automation"}

	for _, r := range results {
		require.Emptyf(t, r.NonRelatedTxHashes,
			"run %d: clear coding transactions must not be non_related; got %v",
			r.RunIndex, r.NonRelatedTxHashes)
		for _, label := range mustAppear {
			_, ok := r.SubdomainsFrequencies[label]
			require.Truef(t, ok,
				"run %d: label %q must appear in finalized frequencies; got %v",
				r.RunIndex, label, r.SubdomainsFrequencies)
		}
		for _, label := range mustNotAppear {
			_, ok := r.SubdomainsFrequencies[label]
			require.Falsef(t, ok,
				"run %d: label %q must not appear in finalized frequencies; got %v",
				r.RunIndex, label, r.SubdomainsFrequencies)
		}
	}
}
