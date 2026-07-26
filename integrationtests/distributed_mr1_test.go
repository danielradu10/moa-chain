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

	"moa-chain/agent/httpclient"
	"moa-chain/data"
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
	Timestamp             string                 `json:"timestamp"`
	MiniRound             string                 `json:"mini_round"`
	RoundNumber           uint64                 `json:"round_number"`
	NumValidators         int                    `json:"num_validators"`
	CommitteeStrategy     string                 `json:"committee_strategy"`
	Temperatures          []float64              `json:"temperatures"`
	DurationSeconds       float64                `json:"duration_seconds"`
	Passed                bool                   `json:"passed"`
	SubdomainsFrequencies data.SubdomainsFrequency `json:"subdomains_frequencies,omitempty"`
	NonRelatedCount       int                    `json:"non_related_count,omitempty"`
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
