//go:build integration

package integrationtests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/data"
	"moa-chain/testscommon"
)

// ── Cluster config ─────────────────────────────────────────────────────────────

type clusterConfig struct {
	Model             string         `json:"model"`
	Port              int            `json:"port"`
	CommitteeStrategy string         `json:"committeeStrategy"`
	Agents            []clusterAgent `json:"agents"`
}

type clusterAgent struct {
	Machine     string  `json:"machine"`
	URL         string  `json:"url"`
	Temperature float64 `json:"temperature"`
	Model       string  `json:"model"`
}

func loadClusterConfig(t *testing.T) clusterConfig {
	t.Helper()
	path := os.Getenv("MOA_CLUSTER_CONFIG")
	if path == "" {
		t.Skip("MOA_CLUSTER_CONFIG not set — skipping distributed test")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	var cfg clusterConfig
	require.NoError(t, json.Unmarshal(raw, &cfg))
	return cfg
}

func pingClusterOrSkip(t *testing.T, cfg clusterConfig) {
	t.Helper()
	for _, a := range cfg.Agents {
		resp, err := http.Get(a.URL + "/health") //nolint:noctx
		if err != nil {
			t.Skipf("cluster agent %s unreachable at %s: %v", a.Machine, a.URL, err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Skipf("cluster agent %s at %s returned HTTP %d", a.Machine, a.URL, resp.StatusCode)
			return
		}
	}
}

// ── Results persistence ────────────────────────────────────────────────────────

type distributedRunResult struct {
	Timestamp             string                   `json:"timestamp"`
	MiniRound             string                   `json:"mini_round"`
	RoundNumber           uint64                   `json:"round_number"`
	NumValidators         int                      `json:"num_validators"`
	CommitteeStrategy     string                   `json:"committee_strategy"`
	Temperatures          []float64                `json:"temperatures"`
	AgentModels           map[string]string        `json:"agent_models"`
	DurationSeconds       float64                  `json:"duration_seconds"`
	Passed                bool                     `json:"passed"`
	SubdomainsFrequencies data.SubdomainsFrequency `json:"subdomains_frequencies,omitempty"`
	NonRelatedCount       int                      `json:"non_related_count,omitempty"`
	NumByzantine          int                      `json:"num_byzantine,omitempty"`
	ByzantineLabel        string                   `json:"byzantine_label,omitempty"`
}

func saveDistributedResults(t *testing.T, result distributedRunResult) {
	t.Helper()
	dir := os.Getenv("MOA_TEST_RESULTS_DIR")
	if dir == "" {
		return
	}
	filename := fmt.Sprintf("distributed-%s-%s.json",
		result.MiniRound, time.Now().UTC().Format("20060102T150405"))
	path := filepath.Join(dir, filename)
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Logf("[results] failed to marshal JSON: %v", err)
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Logf("[results] failed to write %s: %v", path, err)
		return
	}
	t.Logf("[results] saved to %s", path)
}

func clusterTemperatures(cfg clusterConfig) []float64 {
	temps := make([]float64, len(cfg.Agents))
	for i, a := range cfg.Agents {
		temps[i] = a.Temperature
	}
	return temps
}

func clusterAgentModels(cfg clusterConfig) map[string]string {
	models := make(map[string]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		models[fmt.Sprintf("validator-%d", i+1)] = a.Model
	}
	return models
}

// ── Round runner ───────────────────────────────────────────────────────────────

// runDistributedMR1Round executes a full MR1 consensus round across the cluster.
// Each validator calls its own dedicated agent at a distinct temperature.
// All labeling is done by real LLM calls — nothing is hardcoded.
func runDistributedMR1Round(t *testing.T, cfg clusterConfig, roundNumber uint64) *data.BlockOnChain {
	t.Helper()

	n := len(cfg.Agents)
	publicKeys, privateKeys := generateScenarioKeys(t, n)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := realAgentMR2Transactions()

	inboxes := make([]chan data.RoundEvent, n)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 256)
	}

	nodes := make([]*integrationTestNode, n)
	for i, entry := range cfg.Agents {
		agentClient := httpclient.New(httpclient.Config{
			BaseURL:             entry.URL,
			TimeoutSeconds:      720,
			LabelPromptVersion:  "labeler_v3",
			AnswerPromptVersion: "answerer_v1",
		})
		nodes[i] = createNode(
			t,
			fmt.Sprintf("validator-%d", i+1),
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			agentClient,
			true, // stopAfterMiniRoundOne
			0,
		)
	}

	errCh := make(chan error, 256)
	for _, node := range nodes {
		cn := node
		go func() {
			for err := range cn.loop.Errors() {
				errCh <- err
			}
		}()
		go func(nd *integrationTestNode) { nd.loop.Run() }(node)
	}

	mr1Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundOne)}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: mr1Key}
	}
	t.Logf("[distributed-mr1] round %d — %d validators, strategy=%s, temperatures=%v",
		roundNumber, n, cfg.CommitteeStrategy, clusterTemperatures(cfg))

	roundStart := time.Now()
	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return false
		default:
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Minute, 5*time.Second)

	roundDuration := time.Since(roundStart)
	t.Logf("[distributed-mr1] round completed in %s", roundDuration.Round(time.Millisecond))

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
		require.NoError(t, err)
		require.Equal(t, block.SubdomainsFrequencies, other.SubdomainsFrequencies)
		require.Equal(t, block.NonRelatedTransactionHashes, other.NonRelatedTransactionHashes)
	}

	t.Logf("[distributed-mr1] SubdomainsFrequencies: %v", block.SubdomainsFrequencies)
	t.Logf("[distributed-mr1] NonRelatedCount: %d", len(block.NonRelatedTransactionHashes))

	saveDistributedResults(t, distributedRunResult{
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
		MiniRound:             "mr1",
		RoundNumber:           roundNumber,
		NumValidators:         n,
		CommitteeStrategy:     cfg.CommitteeStrategy,
		Temperatures:          clusterTemperatures(cfg),
		AgentModels:           clusterAgentModels(cfg),
		DurationSeconds:       roundDuration.Seconds(),
		Passed:                true,
		SubdomainsFrequencies: block.SubdomainsFrequencies,
		NonRelatedCount:       len(block.NonRelatedTransactionHashes),
	})

	return block
}

// ── Tests ──────────────────────────────────────────────────────────────────────

// TestDistributedMR1_AllAgentsConverge runs a full MR1 consensus round across
// the 10-machine cluster. Each validator calls its own dedicated Ollama instance
// at a distinct temperature (0.30–0.75), testing whether real label diversity
// still converges to an agreed subdomain frequency map.
//
// Skipped automatically when MOA_CLUSTER_CONFIG is not set.
//
// Run with: make test-distributed-mr1
func TestDistributedMR1_AllAgentsConverge(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)
	runDistributedMR1Round(t, cfg, 30)
}

// ── Byzantine distributed test ─────────────────────────────────────────────────

// pingHonestAgentsOrSkip pings only the agents that will run real LLM calls,
// skipping the first numByzantine entries which are replaced by in-process stubs.
func pingHonestAgentsOrSkip(t *testing.T, cfg clusterConfig, numByzantine int) {
	t.Helper()
	for _, a := range cfg.Agents[numByzantine:] {
		resp, err := http.Get(a.URL + "/health") //nolint:noctx
		if err != nil {
			t.Skipf("cluster agent %s unreachable at %s: %v", a.Machine, a.URL, err)
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Skipf("cluster agent %s at %s returned HTTP %d", a.Machine, a.URL, resp.StatusCode)
			return
		}
	}
}

// runDistributedMR1ByzantineRound runs one MR1 round where the first
// numByzantine validators use an in-process stub that immediately returns
// byzantineLabel for every transaction, and the remaining validators call
// real cluster agents with LLM inference.
//
// The test asserts that byzantineLabel does not appear in the finalized
// SubdomainsFrequencies map, confirming the protocol rejects labels that
// cannot reach the Q = ⌊2G/3⌋ + 1 quorum threshold.
func runDistributedMR1ByzantineRound(
	t *testing.T,
	cfg clusterConfig,
	roundNumber uint64,
	numByzantine int,
	byzantineLabel string,
) *data.BlockOnChain {
	t.Helper()

	n := len(cfg.Agents)
	publicKeys, privateKeys := generateScenarioKeys(t, n)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := realAgentMR2Transactions()

	inboxes := make([]chan data.RoundEvent, n)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 256)
	}

	nodes := make([]*integrationTestNode, n)
	for i, entry := range cfg.Agents {
		var agentClient agent.BatchAgent
		if i < numByzantine {
			label := byzantineLabel
			agentClient = &testscommon.LabelerStub{
				LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
					results := make([]agent.LabelResult, len(txs))
					for j, tx := range txs {
						results[j] = agent.LabelResult{TxHash: tx.GetTxHash(), Labels: []string{label}}
					}
					return results, nil
				},
			}
		} else {
			agentClient = httpclient.New(httpclient.Config{
				BaseURL:             entry.URL,
				TimeoutSeconds:      720,
				LabelPromptVersion:  "labeler_v3",
				AnswerPromptVersion: "answerer_v1",
			})
		}
		nodes[i] = createNode(
			t,
			fmt.Sprintf("validator-%d", i+1),
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			agentClient,
			true,
			0,
		)
	}

	errCh := make(chan error, 256)
	for _, node := range nodes {
		cn := node
		go func() {
			for err := range cn.loop.Errors() {
				errCh <- err
			}
		}()
		go func(nd *integrationTestNode) { nd.loop.Run() }(node)
	}

	mr1Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundOne)}
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: mr1Key}
	}
	t.Logf("[distributed-mr1-byzantine] round %d — %d validators (%d byzantine label=%q), strategy=%s",
		roundNumber, n, numByzantine, byzantineLabel, cfg.CommitteeStrategy)

	roundStart := time.Now()
	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return false
		default:
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Minute, 5*time.Second)

	roundDuration := time.Since(roundStart)
	t.Logf("[distributed-mr1-byzantine] round completed in %s", roundDuration.Round(time.Millisecond))

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
		require.NoError(t, err)
		require.Equal(t, block.SubdomainsFrequencies, other.SubdomainsFrequencies)
		require.Equal(t, block.NonRelatedTransactionHashes, other.NonRelatedTransactionHashes)
	}

	_, hasByzantineLabel := block.SubdomainsFrequencies[byzantineLabel]
	require.Falsef(t, hasByzantineLabel,
		"byzantine label %q must not appear in finalized frequencies: only %d/%d votes, below quorum Q=%d; got %v",
		byzantineLabel, numByzantine, n, 2*n/3+1, block.SubdomainsFrequencies)

	t.Logf("[distributed-mr1-byzantine] SubdomainsFrequencies: %v", block.SubdomainsFrequencies)
	t.Logf("[distributed-mr1-byzantine] NonRelatedCount: %d", len(block.NonRelatedTransactionHashes))

	saveDistributedResults(t, distributedRunResult{
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
		MiniRound:             "mr1",
		RoundNumber:           roundNumber,
		NumValidators:         n,
		CommitteeStrategy:     cfg.CommitteeStrategy,
		Temperatures:          clusterTemperatures(cfg),
		AgentModels:           clusterAgentModels(cfg),
		DurationSeconds:       roundDuration.Seconds(),
		Passed:                true,
		SubdomainsFrequencies: block.SubdomainsFrequencies,
		NonRelatedCount:       len(block.NonRelatedTransactionHashes),
		NumByzantine:          numByzantine,
		ByzantineLabel:        byzantineLabel,
	})

	return block
}

// TestDistributedMR1_Byzantine_K3_CorrectLabelSurvives verifies that with k=3
// Byzantine validators (exactly f, the BFT tolerance of the 10-node committee),
// the correct labels still reach quorum and the Byzantine label is excluded.
//
// Setup: validators 0–2 (moa-chain-0 to moa-chain-2) use an in-process stub
// that immediately returns "ml_ai_engineering" for every transaction. Validators
// 3–9 call real cluster agents with LLM inference.
//
// Math: G=10, Q=⌊2·10/3⌋+1=7, f=3. Byzantine validators cast 3 votes for
// "ml_ai_engineering" — below Q=7, so it is excluded. Honest validators provide
// 7 votes for the correct labels — exactly Q, so they are retained.
//
// Run with: make test-distributed-mr1-byzantine-k3
func TestDistributedMR1_Byzantine_K3_CorrectLabelSurvives(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingHonestAgentsOrSkip(t, cfg, 3)
	block := runDistributedMR1ByzantineRound(t, cfg, 40, 3, "ml_ai_engineering")
	t.Logf("[distributed-mr1-byzantine] final frequencies: %v", block.SubdomainsFrequencies)
}

// ── Generalized round runner ───────────────────────────────────────────────────

// runDistributedMR1RoundWith runs a full MR1 round with a caller-supplied
// transaction set. miniRoundLabel is used in log lines and in the saved JSON
// filename (distributed-<miniRoundLabel>-<timestamp>.json).
//
// The function asserts that all nodes finalize the round and agree on the same
// SubdomainsFrequencies and NonRelatedTransactionHashes. Test-specific
// assertions (e.g. exact NonRelatedCount) are left to the caller.
func runDistributedMR1RoundWith(
	t *testing.T,
	cfg clusterConfig,
	roundNumber uint64,
	transactions []data.Transaction,
	miniRoundLabel string,
) *data.BlockOnChain {
	t.Helper()

	n := len(cfg.Agents)
	publicKeys, privateKeys := generateScenarioKeys(t, n)
	registeredValidators := createScenarioValidators(publicKeys)

	inboxes := make([]chan data.RoundEvent, n)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 256)
	}

	nodes := make([]*integrationTestNode, n)
	for i, entry := range cfg.Agents {
		agentClient := httpclient.New(httpclient.Config{
			BaseURL:             entry.URL,
			TimeoutSeconds:      720,
			LabelPromptVersion:  "labeler_v3",
			AnswerPromptVersion: "answerer_v1",
		})
		nodes[i] = createNode(
			t,
			fmt.Sprintf("validator-%d", i+1),
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			agentClient,
			true,
			0,
		)
	}

	errCh := make(chan error, 256)
	for _, node := range nodes {
		cn := node
		go func() {
			for err := range cn.loop.Errors() {
				errCh <- err
			}
		}()
		go func(nd *integrationTestNode) { nd.loop.Run() }(node)
	}

	mr1Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundOne)}
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: mr1Key}
	}
	t.Logf("[%s] round %d — %d validators, strategy=%s, temperatures=%v",
		miniRoundLabel, roundNumber, n, cfg.CommitteeStrategy, clusterTemperatures(cfg))

	roundStart := time.Now()
	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return false
		default:
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Minute, 5*time.Second)

	roundDuration := time.Since(roundStart)
	t.Logf("[%s] round completed in %s", miniRoundLabel, roundDuration.Round(time.Millisecond))

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
		require.NoError(t, err)
		require.Equal(t, block.SubdomainsFrequencies, other.SubdomainsFrequencies)
		require.Equal(t, block.NonRelatedTransactionHashes, other.NonRelatedTransactionHashes)
	}

	t.Logf("[%s] SubdomainsFrequencies: %v", miniRoundLabel, block.SubdomainsFrequencies)
	t.Logf("[%s] NonRelatedCount: %d", miniRoundLabel, len(block.NonRelatedTransactionHashes))

	saveDistributedResults(t, distributedRunResult{
		Timestamp:             time.Now().UTC().Format(time.RFC3339),
		MiniRound:             miniRoundLabel,
		RoundNumber:           roundNumber,
		NumValidators:         n,
		CommitteeStrategy:     cfg.CommitteeStrategy,
		Temperatures:          clusterTemperatures(cfg),
		AgentModels:           clusterAgentModels(cfg),
		DurationSeconds:       roundDuration.Seconds(),
		Passed:                true,
		SubdomainsFrequencies: block.SubdomainsFrequencies,
		NonRelatedCount:       len(block.NonRelatedTransactionHashes),
	})

	return block
}

// ── Transaction factories ──────────────────────────────────────────────────────

// distributedNonRelatedTransactions returns a mixed set: one clearly in-scope
// coding prompt and two prompts that are entirely off-topic for the platform.
// Expected outcome: NonRelatedCount == 2, frequency map has only the coding subdomain.
func distributedNonRelatedTransactions() []data.Transaction {
	return []data.Transaction{
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "alice", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 90, Timestamp: 1,
			TxHash: "distributed-nr-tx-001",
			Prompt: "What is the main benefit of unit tests?",
		}),
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "bob", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 80, Timestamp: 2,
			TxHash: "distributed-nr-tx-002",
			Prompt: "What is the capital of France?",
		}),
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "carol", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 70, Timestamp: 3,
			TxHash: "distributed-nr-tx-003",
			Prompt: "How do you bake a chocolate cake?",
		}),
	}
}

// distributedAmbiguousTransactions returns three prompts that each straddle the
// boundary between two well-known subdomains. Used by the frequency-variance test.
//
//   tx1 — "deterministic ordering in consensus" → systems_programming / blockchain_engineering
//   tx2 — "hash functions protecting blockchain data" → security / blockchain_engineering
//   tx3 — "smart contract reentrancy vulnerabilities" → security / blockchain_engineering
func distributedAmbiguousTransactions() []data.Transaction {
	return []data.Transaction{
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "alice", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 90, Timestamp: 1,
			TxHash: "distributed-amb-tx-001",
			Prompt: "Why does deterministic ordering matter in consensus?",
		}),
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "bob", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 80, Timestamp: 2,
			TxHash: "distributed-amb-tx-002",
			Prompt: "How does a hash function protect data stored on a blockchain?",
		}),
		createTransactionFromFixture(miniRoundOneTransactionFixture{
			Sender: "carol", Receiver: "moa-chain", Nonce: 0,
			TransferredValue: 0, Tip: 70, Timestamp: 3,
			TxHash: "distributed-amb-tx-003",
			Prompt: "What makes a smart contract vulnerable to reentrancy attacks?",
		}),
	}
}

// ── Test 3: Non-related transactions ──────────────────────────────────────────

// TestDistributedMR1_NonRelated_Convergence verifies that prompts with no
// relation to software engineering are excluded from the subdomain frequency map
// and appear in NonRelatedTransactionHashes instead.
//
// Transactions: 1 coding (unit tests) + 2 off-topic (geography, cooking).
// Assertions:
//   - NonRelatedCount == 2 — both off-topic prompts rejected from the domain
//   - SubdomainsFrequencies is non-empty — the coding prompt still produces a label
//   - All 10 nodes agree on the same map and non-related set
//
// Run with: make test-distributed-mr1-non-related
func TestDistributedMR1_NonRelated_Convergence(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)

	block := runDistributedMR1RoundWith(t, cfg, 50, distributedNonRelatedTransactions(), "mr1-non-related")

	require.Equal(t, 2, len(block.NonRelatedTransactionHashes),
		"expected exactly 2 non-related transactions (geography + cooking); got %d: %v",
		len(block.NonRelatedTransactionHashes), block.NonRelatedTransactionHashes)
	require.NotEmpty(t, block.SubdomainsFrequencies,
		"coding transaction (unit tests) must produce at least one subdomain in the frequency map")
}

// ── Test 4: Ambiguous / borderline prompts ────────────────────────────────────

// TestDistributedMR1_Ambiguous_FrequencyVariance measures how the 10-node
// committee classifies prompts that sit on the boundary between two subdomains.
//
// The test only asserts that consensus is reached (round finalizes, all nodes
// agree). Run the trials target to observe how the frequency map varies across
// repeated rounds — that variance is the research output.
//
// Run with: make test-distributed-mr1-ambiguous
// Repeated:  make test-distributed-mr1-ambiguous-trials N=10
func TestDistributedMR1_Ambiguous_FrequencyVariance(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)

	block := runDistributedMR1RoundWith(t, cfg, 60, distributedAmbiguousTransactions(), "mr1-ambiguous")
	t.Logf("[distributed-mr1-ambiguous] final frequency map: %v", block.SubdomainsFrequencies)
}
