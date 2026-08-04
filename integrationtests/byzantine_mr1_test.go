//go:build integration

package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
)

// corruptedAgent always returns ["security"] for every transaction,
// simulating a Byzantine validator that consistently mislabels.
type corruptedAgent struct{}

var _ agent.BatchAgent = (*corruptedAgent)(nil)

func (a *corruptedAgent) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	results := make([]agent.LabelResult, len(txs))
	for i, tx := range txs {
		results[i] = agent.LabelResult{TxHash: tx.GetTxHash(), Labels: []string{"security"}}
	}
	return results, nil
}

func (a *corruptedAgent) AnswerBatch(_ []data.Transaction) ([]agent.AnswerResult, error) {
	return nil, errors.New("corruptedAgent: AnswerBatch not used in MR1")
}

func (a *corruptedAgent) JudgeTransactionAnswers(_ agent.AnswerJudgeRequest) (string, error) {
	return "", errors.New("corruptedAgent: JudgeTransactionAnswers not used in MR1")
}

// byzantineRoundOutcome is returned by tryRunMR1RoundWithByzantine.
// It never causes t.FailNow — outcomes are observations, not assertions.
type byzantineRoundOutcome struct {
	Reached        bool
	FinalizedCount int
	Block          *data.BlockOnChain
	Duration       time.Duration
	NodeErrors     []string
}

// tryRunMR1RoundWithByzantine runs one MR1 round where the first numCorrupted
// validators use corruptedAgent (always returns "security") and the rest use
// honestAgent. It never calls t.FailNow: it returns an outcome and always sends
// StopEvent to all nodes before returning, even on timeout.
func tryRunMR1RoundWithByzantine(
	t *testing.T,
	honestAgent agent.BatchAgent,
	transactions []data.Transaction,
	roundNumber uint64,
	numCorrupted int,
) byzantineRoundOutcome {
	t.Helper()

	const numValidators = 20

	publicKeys := make([][]byte, 0, numValidators)
	privateKeys := make([][]byte, 0, numValidators)
	for i := 0; i < numValidators; i++ {
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		publicKeys = append(publicKeys, pubKey)
		privateKeys = append(privateKeys, privKey)
	}

	registeredValidators := createValidators(publicKeys)

	inboxes := make([]chan data.RoundEvent, numValidators)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 128)
	}

	defer func() {
		for _, inbox := range inboxes {
			select {
			case inbox <- data.RoundEvent{Type: data.StopEvent}:
			default:
			}
		}
	}()

	var errMu sync.Mutex
	var nodeErrors []string

	nodes := make([]*integrationTestNode, numValidators)
	for i := 0; i < numValidators; i++ {
		var ag agent.BatchAgent
		if i < numCorrupted {
			ag = &corruptedAgent{}
		} else {
			ag = honestAgent
		}
		validatorID := fmt.Sprintf("validator-%d", i+1)
		nodes[i] = createNode(
			t,
			validatorID,
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			ag,
			true,
			7*time.Minute,
		)
	}

	for _, node := range nodes {
		cur := node
		go func() {
			for err := range cur.loop.Errors() {
				errMu.Lock()
				nodeErrors = append(nodeErrors, err.Error())
				errMu.Unlock()
			}
		}()
	}
	for _, node := range nodes {
		cur := node
		go func() { cur.loop.Run() }()
	}

	roundKey := data.RoundKey{
		Epoch:     0,
		Round:     roundNumber,
		MiniRound: uint64(data.MiniRoundOne),
	}

	roundStart := time.Now()
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: roundKey}
	}
	t.Logf("[%s] byzantine round %d started — %d validators (%d corrupted), %d tx",
		roundStart.Format(time.TimeOnly), roundNumber, numValidators, numCorrupted, len(transactions))

	timeout := time.NewTimer(10 * time.Minute)
	defer timeout.Stop()
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()

	countFinalized := func() (int, *data.BlockOnChain) {
		count := 0
		var first *data.BlockOnChain
		for _, node := range nodes {
			b, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
			if err == nil && b != nil {
				count++
				if first == nil {
					first = b
				}
			}
		}
		return count, first
	}

	for {
		select {
		case <-timeout.C:
			finalized, first := countFinalized()
			errMu.Lock()
			errs := append([]string(nil), nodeErrors...)
			errMu.Unlock()
			t.Logf("[%s] byzantine round %d timed out — %d/%d validators finalized",
				time.Now().Format(time.TimeOnly), roundNumber, finalized, numValidators)
			return byzantineRoundOutcome{
				Reached:        false,
				FinalizedCount: finalized,
				Block:          first,
				Duration:       time.Since(roundStart),
				NodeErrors:     errs,
			}

		case <-tick.C:
			finalized, first := countFinalized()
			if finalized == numValidators {
				errMu.Lock()
				errs := append([]string(nil), nodeErrors...)
				errMu.Unlock()
				t.Logf("[%s] byzantine round %d — all %d finalized in %s",
					time.Now().Format(time.TimeOnly), roundNumber, numValidators,
					time.Since(roundStart).Round(time.Millisecond))
				return byzantineRoundOutcome{
					Reached:        true,
					FinalizedCount: finalized,
					Block:          first,
					Duration:       time.Since(roundStart),
					NodeErrors:     errs,
				}
			}
			if finalized > 0 {
				t.Logf("[%s] %d/%d finalized (+%s)",
					time.Now().Format(time.TimeOnly), finalized, numValidators,
					time.Since(roundStart).Round(time.Millisecond))
			}
		}
	}
}

// byzantineRunResult is the record persisted after each Byzantine test run.
type byzantineRunResult struct {
	TestGroup             string                   `json:"testGroup"`
	RunIndex              int                      `json:"runIndex"`
	RoundNumber           uint64                   `json:"roundNumber"`
	Timestamp             string                   `json:"timestamp"`
	RoundDurationMs       int64                    `json:"roundDurationMs"`
	NumValidators         int                      `json:"numValidators"`
	NumCorrupted          int                      `json:"numCorrupted"`
	NumTransactions       int                      `json:"numTransactions"`
	ConsensusReached      bool                     `json:"consensusReached"`
	FinalizedCount        int                      `json:"finalizedCount"`
	SubdomainsFrequencies data.SubdomainsFrequency `json:"subdomainsFrequencies,omitempty"`
	NodeErrors            []string                 `json:"nodeErrors,omitempty"`
}

func appendByzantineRunResult(t *testing.T, result byzantineRunResult) {
	t.Helper()

	encoded, err := json.Marshal(result)
	require.NoError(t, err)

	path := filepath.Join("testData", "miniround1", "results", "byzantine_runs_results.jsonl")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	defer func() { require.NoError(t, f.Close()) }()

	_, err = f.Write(append(encoded, '\n'))
	require.NoError(t, err)
}

func runByzantineRepeatedRounds(
	t *testing.T,
	testGroup string,
	transactions []data.Transaction,
	baseRound uint64,
	numCorrupted int,
) []byzantineRunResult {
	t.Helper()

	n := repeatedRunCount()
	results := make([]byzantineRunResult, 0, n)

	timingLog, closeLog := createTimingLogger(t)
	defer closeLog()

	wrapped := &timingAgent{t: t, inner: realAgentClient(), logger: timingLog}

	for i := 0; i < n; i++ {
		roundNumber := baseRound + uint64(i)

		t.Logf("=== byzantine run %d/%d  group=%s  round=%d  corrupted=%d ===",
			i+1, n, testGroup, roundNumber, numCorrupted)
		timingLog.Info("run_start",
			"run", i+1, "total", n, "group", testGroup,
			"round", roundNumber, "corrupted", numCorrupted)

		outcome := tryRunMR1RoundWithByzantine(t, wrapped, transactions, roundNumber, numCorrupted)

		var freqs data.SubdomainsFrequency
		if outcome.Block != nil {
			freqs = outcome.Block.SubdomainsFrequencies
		}

		result := byzantineRunResult{
			TestGroup:             testGroup,
			RunIndex:              i + 1,
			RoundNumber:           roundNumber,
			Timestamp:             time.Now().UTC().Format(time.RFC3339Nano),
			RoundDurationMs:       outcome.Duration.Milliseconds(),
			NumValidators:         20,
			NumCorrupted:          numCorrupted,
			NumTransactions:       len(transactions),
			ConsensusReached:      outcome.Reached,
			FinalizedCount:        outcome.FinalizedCount,
			SubdomainsFrequencies: freqs,
			NodeErrors:            outcome.NodeErrors,
		}
		results = append(results, result)
		appendByzantineRunResult(t, result)

		t.Logf("=== run %d/%d done — reached=%v finalized=%d/20 duration=%s frequencies=%v ===",
			i+1, n, outcome.Reached, outcome.FinalizedCount,
			outcome.Duration.Round(time.Millisecond), freqs)
		timingLog.Info("run_end",
			"run", i+1,
			"reached", outcome.Reached,
			"finalized_count", outcome.FinalizedCount,
			"duration", outcome.Duration.Round(time.Millisecond),
			"frequencies", freqs,
			"node_errors", len(outcome.NodeErrors),
		)
	}

	return results
}

// createSingleMlAiTransaction returns the backpropagation transaction used for
// Byzantine fault tolerance tests. A single transaction keeps the test focused:
// the only question is whether the label survives Byzantine validators or not.
func createSingleMlAiTransaction() []data.Transaction {
	return []data.Transaction{
		createTransaction(
			"alice", 0,
			"How does backpropagation work in a neural network and why is the chain rule needed?",
			80, 301, nil,
		),
	}
}

// TestMiniRoundOne_RealAgent_Byzantine_K3_CorrectLabelSurvives verifies that
// with k=3 corrupted validators (exactly f, the BFT tolerance of the 20-node
// network), consensus always converges on the correct label.
//
// Math: G=10, Q=7, f=3. Worst case: all 3 corrupted land in the consensus
// group → 7 honest members remain, which is exactly Q. The correct label
// always reaches quorum; "security" never does (max 3 votes < Q=7).
//
// Run with: make test-realagent-mr1-byzantine-k3
func TestMiniRoundOne_RealAgent_Byzantine_K3_CorrectLabelSurvives(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createSingleMlAiTransaction()
	results := runByzantineRepeatedRounds(t, "Byzantine_K3", transactions, 300, 3)

	for _, r := range results {
		require.Truef(t, r.ConsensusReached,
			"run %d: consensus must always be reached with k=3 corrupted (f=3 BFT bound); got finalized=%d/20",
			r.RunIndex, r.FinalizedCount)
		_, ok := r.SubdomainsFrequencies["ml_ai_engineering"]
		require.Truef(t, ok,
			"run %d: ml_ai_engineering must be in finalized frequencies; got %v",
			r.RunIndex, r.SubdomainsFrequencies)
		_, bad := r.SubdomainsFrequencies["security"]
		require.Falsef(t, bad,
			"run %d: security must not reach quorum with only k=3 corrupted votes; got %v",
			r.RunIndex, r.SubdomainsFrequencies)
	}
}

// TestMiniRoundOne_RealAgent_Byzantine_K6_ProtocolDegrades verifies that with
// k=6 corrupted validators (2×f), the protocol degrades: rounds in which 4+
// corrupted validators land in the consensus group (P≈31.5% per round) fail to
// finalize because neither "security" nor "ml_ai_engineering" reaches Q=7.
//
// This test does not make hard correctness assertions — it records every
// outcome so the failure rate can be compared to the theoretical prediction.
// Expected over 5 rounds: ~1–2 failures (P(≥1 failure)≈87%).
//
// Run with: make test-realagent-mr1-byzantine-k6
func TestMiniRoundOne_RealAgent_Byzantine_K6_ProtocolDegrades(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createSingleMlAiTransaction()
	results := runByzantineRepeatedRounds(t, "Byzantine_K6", transactions, 400, 6)

	reached := 0
	for _, r := range results {
		if r.ConsensusReached {
			reached++
		}
		t.Logf("run %d: reached=%v finalized=%d/20 frequencies=%v",
			r.RunIndex, r.ConsensusReached, r.FinalizedCount, r.SubdomainsFrequencies)
	}
	t.Logf("k=6 summary: %d/%d rounds reached consensus (theory predicts ~%.1f)",
		reached, len(results), float64(len(results))*0.685)
}
