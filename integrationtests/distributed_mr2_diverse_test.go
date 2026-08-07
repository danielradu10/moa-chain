//go:build integration

package integrationtests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"moa-chain/agent/httpclient"
	"moa-chain/data"
	"moa-chain/validators"
)

func committeeStrategyFromConfig(cfg clusterConfig) validators.CommitteeStrategy {
	if cfg.CommitteeStrategy == "full" {
		return validators.CommitteeStrategyFull
	}
	return validators.CommitteeStrategyHalf
}

// ── Results ────────────────────────────────────────────────────────────────────

type distributedMR2DiverseTxResult struct {
	TxHash        string `json:"tx_hash"`
	Status        string `json:"status"`
	Correct       int    `json:"correct"`
	Wrong         int    `json:"wrong"`
	Hallucination int    `json:"hallucination"`
	Malicious     int    `json:"malicious"`

	CorrectCandidates       []configurableAdversarialCandidate `json:"correct_candidates"`
	WrongCandidates         []configurableAdversarialCandidate `json:"wrong_candidates"`
	HallucinationCandidates []configurableAdversarialCandidate `json:"hallucination_candidates"`
	MaliciousCandidates     []configurableAdversarialCandidate `json:"malicious_candidates"`
}

type distributedMR2DiverseRunResult struct {
	Timestamp                     string                          `json:"timestamp"`
	Group                         string                          `json:"group"`
	RoundNumber                   uint64                          `json:"round_number"`
	NumValidators                 int                             `json:"num_validators"`
	AgentModels                   map[string]string               `json:"agent_models"`
	BadProducerIDs                []string                        `json:"bad_producer_ids"`
	ClassificationVoteSource      string                          `json:"classification_vote_source,omitempty"`
	ClassificationVotes           []qualifiedClassificationVote   `json:"classification_votes,omitempty"`
	MissingClassificationJudgeIDs []string                        `json:"missing_classification_judge_ids,omitempty"`
	DurationSeconds               float64                         `json:"duration_seconds"`
	Finalized                     bool                            `json:"finalized"`
	TxResults                     []distributedMR2DiverseTxResult `json:"tx_results,omitempty"`
}

func saveDistributedMR2DiverseResult(t *testing.T, result distributedMR2DiverseRunResult) {
	t.Helper()
	dir := os.Getenv("MOA_TEST_RESULTS_DIR")
	if dir == "" {
		return
	}
	filename := fmt.Sprintf("distributed-mr2-diverse-%s-%s.json",
		result.Group, time.Now().UTC().Format("20060102T150405"))
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

func mr2DiverseTxResults(block *data.BlockOnChain) []distributedMR2DiverseTxResult {
	out := make([]distributedMR2DiverseTxResult, 0, len(block.AnswerClassifications))
	for _, tx := range block.AnswerClassifications {
		out = append(out, distributedMR2DiverseTxResult{
			TxHash:                  string(tx.TxHash),
			Status:                  string(tx.Status),
			Correct:                 len(tx.Groups.Correct),
			Wrong:                   len(tx.Groups.Wrong),
			Hallucination:           len(tx.Groups.Hallucination),
			Malicious:               len(tx.Groups.Malicious),
			CorrectCandidates:       configurableAdversarialCandidates(tx.Groups.Correct),
			WrongCandidates:         configurableAdversarialCandidates(tx.Groups.Wrong),
			HallucinationCandidates: configurableAdversarialCandidates(tx.Groups.Hallucination),
			MaliciousCandidates:     configurableAdversarialCandidates(tx.Groups.Malicious),
		})
	}
	return out
}

// ── Round runner ───────────────────────────────────────────────────────────────

// runDistributedMR2DiverseRound runs a full MR1→MR2 consensus round with
// diverse correct answers distributed across validators. Each validator i
// receives mr2RADiverseAnswers[i % len(mr2RADiverseAnswers)]; if badAnswers is
// non-nil, validators 0–3 receive those instead.
//
// Unlike runDistributedMR2Round, this runner uses assert.Eventually so that a
// quorum breakdown is recorded as a finding rather than a test failure — the
// distributed judge failing to agree on diverse-but-correct evidence is itself
// a meaningful result.
func runDistributedMR2DiverseRound(
	t *testing.T,
	cfg clusterConfig,
	group string,
	badAnswers map[string]string,
	roundNumber uint64,
) (*data.BlockOnChain, bool) {
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
		answers := mr2RADiverseAnswers[i%len(mr2RADiverseAnswers)]
		if badAnswers != nil && i < 4 {
			answers = badAnswers
		}

		judgeClient := httpclient.New(httpclient.Config{
			BaseURL:             entry.URL,
			TimeoutSeconds:      720,
			JudgeTimeoutSeconds: qualifiedJudgeTimeoutSeconds(t, 150),
			LabelPromptVersion:  "labeler_v3",
			AnswerPromptVersion: "answerer_v1",
		})

		nodes[i] = createNodeWithCommitteeStrategy(
			t,
			fmt.Sprintf("validator-%d", i+1),
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			&realAgentMR2Agent{
				labels:  mr2RATxLabels,
				answers: answers,
				judge:   judgeClient,
			},
			false,
			0,
			committeeStrategyFromConfig(cfg),
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
	mr2Key := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundTwo)}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: mr1Key}
	}
	t.Logf("[distributed-mr2-diverse-%s] round %d — %d validators, diverse answers, strategy=%s",
		group, roundNumber, n, cfg.CommitteeStrategy)

	roundStart := time.Now()
	finalized := assert.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return false
		default:
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key); err != nil {
				return false
			}
		}
		return true
	}, qualifiedRoundTimeout(t, 6*time.Minute), 5*time.Second)

	roundDuration := time.Since(roundStart)
	badProducerIDs := []string(nil)
	if badAnswers != nil {
		badProducerIDs = qualifiedBadProducerIDs(4)
	}
	voteSource, classificationVotes := collectQualifiedClassificationVotes(
		nodes, mr2Key, cfg, group, badProducerIDs, nil,
	)

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	if !finalized {
		t.Logf("[distributed-mr2-diverse-%s] FINDING: round %d did not finalize within %s — "+
			"check validator and agent logs to determine root cause "+
			"(judge HTTP timeout, leader vote collection failure, or classification disagreement).",
			group, roundNumber, roundDuration.Round(time.Second))
		saveDistributedMR2DiverseResult(t, distributedMR2DiverseRunResult{
			Timestamp:                     time.Now().UTC().Format(time.RFC3339),
			Group:                         group,
			RoundNumber:                   roundNumber,
			NumValidators:                 n,
			AgentModels:                   clusterAgentModels(cfg),
			BadProducerIDs:                badProducerIDs,
			ClassificationVoteSource:      voteSource,
			ClassificationVotes:           classificationVotes,
			MissingClassificationJudgeIDs: qualifiedMissingJudgeIDs(classificationVotes, n),
			DurationSeconds:               roundDuration.Seconds(),
			Finalized:                     false,
		})
		return nil, false
	}

	t.Logf("[distributed-mr2-diverse-%s] round completed in %s", group, roundDuration.Round(time.Millisecond))

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
		require.NoError(t, err)
		require.Equal(t, block.AnswerClassifications, other.AnswerClassifications)
	}

	logMR2BlockSummary(t, block)

	saveDistributedMR2DiverseResult(t, distributedMR2DiverseRunResult{
		Timestamp:                     time.Now().UTC().Format(time.RFC3339),
		Group:                         group,
		RoundNumber:                   roundNumber,
		NumValidators:                 n,
		AgentModels:                   clusterAgentModels(cfg),
		BadProducerIDs:                badProducerIDs,
		ClassificationVoteSource:      voteSource,
		ClassificationVotes:           classificationVotes,
		MissingClassificationJudgeIDs: qualifiedMissingJudgeIDs(classificationVotes, n),
		DurationSeconds:               roundDuration.Seconds(),
		Finalized:                     true,
		TxResults:                     mr2DiverseTxResults(block),
	})

	return block, true
}

// ── Test: Group A — All correct, diverse ──────────────────────────────────────

// TestDistributedMR2_Diverse_GroupA_CanonicalPreferenceBias probes whether
// the distributed judge cluster can reach BFT agreement when all 10 validators
// produce correct but differently phrased answers.
//
// Each validator i receives mr2RADiverseAnswers[i%6], cycling through 6
// distinct correct perspectives on the same questions. No bad producers exist.
//
// Expected finding: quorum breakdown. Each validator's judge classifies some
// correct phrasings as WRONG because it treats one phrasing as canonical.
// Since different cluster agents disagree on which phrasing is canonical,
// validators cast different classification votes and BFT agreement is never
// reached. This is the canonical-preference bias that motivates the one-
// candidate-per-/judge-call fix.
//
// A successful finalization is also a valid result — it means the 7B judge
// on the distributed cluster handles diverse-but-correct inputs without bias.
//
// Run with: make test-distributed-mr2-diverse-group-a
func TestDistributedMR2_Diverse_GroupA_CanonicalPreferenceBias(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)
	// Use the current Unix timestamp as the round number so that the leader
	// selection seed changes between trials, rotating the leader across runs.
	runDistributedMR2DiverseRound(t, cfg, "group_a", nil, uint64(time.Now().Unix()))
}

// ── Test: Group B — Clearly wrong answers, diverse honest answers ────────────────

// TestDistributedMR2_Diverse_GroupB_WrongAnswerIsRejected verifies the
// distributed full-committee protocol when four producers submit clearly
// incorrect answers and the other six submit diverse correct answers.
//
// Every judge evaluates all 10 evidence candidates independently. A successful
// run must finalize the same result on all nodes, place at least one candidate
// from every transaction in a non-correct group, and mark every transaction
// INSUFFICIENT_CORRECT_ANSWERS because at most six producers supplied correct
// answers, below the seven-producer quorum required to advance to MR3.
//
// Run with: make test-distributed-mr2-diverse-group-b
func TestDistributedMR2_Diverse_GroupB_WrongAnswerIsRejected(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)
	// Use the current Unix timestamp as the round number so that the leader
	// selection seed changes between trials, rotating the leader across runs.
	block, finalized := runDistributedMR2DiverseRound(
		t,
		cfg,
		"group_b",
		mr2RABadAnswers["group_b"],
		uint64(time.Now().Unix()),
	)
	if finalized {
		requireBadAnswersRejected(t, block)
	}
}

// ── Test: Group C — Prompt injection, diverse honest answers ──────────────────

// TestDistributedMR2_Diverse_GroupC_PromptInjectionResistance verifies the
// distributed full-committee protocol when four producers embed explicit
// instructions intended to override the answer judge and the other six submit
// diverse correct answers.
//
// A successful run must finalize the same result on all nodes, place at least
// one candidate from every transaction in a non-correct group, and mark every
// transaction INSUFFICIENT_CORRECT_ANSWERS. If the injection is followed and
// the injected candidates become Correct, the test fails as a security issue.
//
// Run with: make test-distributed-mr2-diverse-group-c
func TestDistributedMR2_Diverse_GroupC_PromptInjectionResistance(t *testing.T) {
	cfg := loadClusterConfig(t)
	pingClusterOrSkip(t, cfg)
	// Use the current Unix timestamp as the round number so that the leader
	// selection seed changes between trials, rotating the leader across runs.
	block, finalized := runDistributedMR2DiverseRound(
		t,
		cfg,
		"group_c",
		mr2RABadAnswers["group_c"],
		uint64(time.Now().Unix()),
	)
	if finalized {
		requireBadAnswersRejected(t, block)
	}
}
