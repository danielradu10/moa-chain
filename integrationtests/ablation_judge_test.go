//go:build integration

package integrationtests

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/consensus/miniround2/classification"
)

// ablationTxFixtures lists the 3 transactions used in all ablation experiments.
// They are the same transactions used in the Group A–F diverse tests.
var ablationTxFixtures = []struct {
	hash   string
	prompt string
}{
	{mr2RATxHash1, "What is the main benefit of unit tests?"},
	{mr2RATxHash2, "Why must validators verify message signatures?"},
	{mr2RATxHash3, "Why does deterministic ordering matter in consensus?"},
}

// invokeSingleCandidateJudge sends one answer to the judge as the only
// candidate in the request. It records the call, logs the result, and returns
// the category. Returns ("", false) on HTTP or parse failure without failing
// the test — a single transient error should not abort an ablation run.
func invokeSingleCandidateJudge(
	t *testing.T,
	tag string,
	run int,
	judgeClient agent.AnswersJudge,
	recorder *mr2Recorder,
	txHash, txPrompt, alias, label, answer string,
) (category string, ok bool) {
	t.Helper()

	input := benchJudgeInput{
		TransactionHash: txHash,
		Prompt:          txPrompt,
		Candidates:      []benchJudgeEntry{{CandidateID: alias, Answer: answer}},
	}
	userPromptBytes, err := json.Marshal(input)
	require.NoError(t, err)

	req := agent.AnswerJudgeRequest{
		SystemPrompt: classification.AnswerJudgeProtocolPrompt,
		UserPrompt:   string(userPromptBytes),
	}

	t0 := time.Now()
	response, callErr := judgeClient.JudgeTransactionAnswers(req)
	dur := time.Since(t0)

	if callErr != nil {
		t.Logf("[%s] run %d tx %q %s (%s) → FAILED (%v)", tag, run, txHash, alias, label, callErr)
		return "", false
	}

	recorder.recordJudgeCall(req, response, dur)

	var parsed benchJudgeOutput
	if parseErr := json.Unmarshal([]byte(response), &parsed); parseErr != nil || len(parsed.Classifications) == 0 {
		t.Logf("[%s] run %d tx %q %s (%s) → parse error: %v", tag, run, txHash, alias, label, parseErr)
		return "", false
	}

	category = parsed.Classifications[0].Category
	t.Logf("[%s] run %d tx %q %s (%s) → %s (%dms)", tag, run, txHash, alias, label, category, dur.Milliseconds())
	return category, true
}

// ── Ablation 1 — Group A (all correct, diverse) ───────────────────────────────

// TestMiniRoundTwo_Ablation1_SingleCandidateJudging is ablation experiment 1
// for the canonical-preference bias investigation.
//
// Baseline observation (Group A diverse, round 30): when 7 semantically diverse
// but equally correct candidate answers are sent to the judge in a single batch
// request, the model marks only 1–2 as CORRECT and flags the rest as WRONG.
// Two competing hypotheses explain this:
//
//  1. Context-driven bias: the model compares candidates against each other
//     (or anchors on the first candidate), picks one as "canonical", and
//     rejects all others as deviating from it.
//
//  2. Parametric bias: the model has a narrow internal representation of the
//     correct answer from pre-training. It compares each candidate against that
//     internal representation independently — no cross-candidate comparison
//     needed — and rejects phrasings that do not closely match its stored form.
//
// Method: each of the 7 candidate answers from the Group A diverse evidence is
// submitted to the judge as the sole candidate in an independent HTTP request.
// System prompt, user-prompt JSON schema, and answer texts are identical to the
// batch test. Only the number of candidates per call changes (1 instead of 7).
// Each candidate is evaluated 3 times to quantify run-to-run variance.
//
// Interpretation guide:
//   - Most single-candidate calls return CORRECT (≥5/7):
//     context-driven bias. Removing the other candidates from the prompt fixes
//     the problem. The architectural solution (one call per candidate) works.
//   - Same 1–2 candidates win regardless of isolation:
//     parametric bias. The model's weights contain a narrow canonical answer;
//     a prompt fix or architectural change cannot overcome this. A larger model
//     or a two-stage generate-then-compare approach is needed.
//
// Run with: make test-realagent-mr2-ablation1
func TestMiniRoundTwo_Ablation1_SingleCandidateJudging(t *testing.T) {
	pingAgentOrSkip(t)

	judgeClient := realAgentClient()
	recorder := newMR2Recorder(t, "ablation_g1/single_candidate", 40)

	// 7 candidates per tx: all 6 distinct perspectives, then perspective 0
	// repeated as the 7th (validator slot 6 wraps: 6 % 6 = 0), matching the
	// composition of a typical Group A diverse evidence quorum.
	buildCandidates := func(txHash string) []struct{ label, answer string } {
		out := make([]struct{ label, answer string }, 0, 7)
		for i, ps := range mr2RADiverseAnswers {
			out = append(out, struct{ label, answer string }{
				label:  fmt.Sprintf("perspective-%d", i+1),
				answer: ps[txHash],
			})
		}
		out = append(out, struct{ label, answer string }{
			label:  "perspective-1(repeat)",
			answer: mr2RADiverseAnswers[0][txHash],
		})
		return out
	}

	const runsPerCandidate = 3
	cooldown := func() { time.Sleep(2 * time.Second) }

	for _, tx := range ablationTxFixtures {
		candidates := buildCandidates(tx.hash)
		t.Logf("[ablation1] tx %q: %d candidates × %d runs = %d calls",
			tx.hash, len(candidates), runsPerCandidate, len(candidates)*runsPerCandidate)

		correctByCandidate := make([]int, len(candidates))

		for run := 1; run <= runsPerCandidate; run++ {
			// Shuffle call order so temporal effects (model warm/cool state,
			// system load) are distributed randomly across candidate types.
			order := rand.Perm(len(candidates))
			for _, i := range order {
				cooldown()
				alias := fmt.Sprintf("candidate-%d", i+1)
				cat, ok := invokeSingleCandidateJudge(t, "ablation1", run, judgeClient, recorder,
					tx.hash, tx.prompt, alias, candidates[i].label, candidates[i].answer)
				if ok && cat == "CORRECT" {
					correctByCandidate[i]++
				}
			}
		}

		atLeastOnce, allRuns := 0, 0
		for _, count := range correctByCandidate {
			if count > 0 {
				atLeastOnce++
			}
			if count == runsPerCandidate {
				allRuns++
			}
		}
		t.Logf("[ablation1] tx %q: per-candidate correct counts across %d runs: %v",
			tx.hash, runsPerCandidate, correctByCandidate)
		t.Logf("[ablation1] tx %q: %d/7 CORRECT in ≥1 run, %d/7 in all %d runs (batch baseline: 1–2/7)",
			tx.hash, atLeastOnce, allRuns, runsPerCandidate)
	}
}

// ── Ablation 2 — Group B (wrong factual answer + diverse) ────────────────────

// TestMiniRoundTwo_Ablation2_SingleCandidateJudging_GroupB tests whether
// single-candidate mode retains correct discriminative behaviour when one of
// the 7 candidates is genuinely wrong.
//
// Motivation: Ablation 1 proved that single-candidate mode accepts all diverse
// correct answers (7/7 CORRECT). A skeptic could still argue that the mode is
// simply permissive — it approves everything indiscriminately. Ablation 2 closes
// that gap by submitting one bad answer alongside the 6 diverse correct ones.
//
// Candidate composition (same as Group B diverse evidence):
//   - candidates 1–6: the 6 distinct correct perspectives from mr2RADiverseAnswers
//   - candidate 7:    the Group B bad answer (clearly wrong factual claims)
//
// Expected outcome:
//   - Candidates 1–6 → CORRECT (diverse correct, evaluated in isolation)
//   - Candidate 7    → WRONG  (bad answer, correctly rejected)
//
// Batch baseline (Group B diverse, round 31): 2–4 CORRECT, 3–5 WRONG per tx.
// The batch result is uninterpretable because canonical-preference bias and
// genuine rejection are mixed — it is impossible to tell how many of the WRONG
// verdicts are false positives vs. correct detections. Single-candidate mode
// separates the two: each verdict is independent.
//
// Run with: make test-realagent-mr2-ablation2
func TestMiniRoundTwo_Ablation2_SingleCandidateJudging_GroupB(t *testing.T) {
	pingAgentOrSkip(t)

	judgeClient := realAgentClient()
	recorder := newMR2Recorder(t, "ablation_g2/single_candidate", 41)

	// 7 candidates per tx: 6 diverse correct perspectives + 1 bad answer.
	buildCandidates := func(txHash string) []struct{ label, answer string } {
		out := make([]struct{ label, answer string }, 0, 7)
		for i, ps := range mr2RADiverseAnswers {
			out = append(out, struct{ label, answer string }{
				label:  fmt.Sprintf("perspective-%d", i+1),
				answer: ps[txHash],
			})
		}
		out = append(out, struct{ label, answer string }{
			label:  "bad",
			answer: mr2RABadAnswers["group_b"][txHash],
		})
		return out
	}

	const (
		runsPerCandidate  = 3
		correctCandidates = 6
		badCandidateIdx   = 6
	)
	cooldown := func() { time.Sleep(2 * time.Second) }

	for _, tx := range ablationTxFixtures {
		candidates := buildCandidates(tx.hash)
		t.Logf("[ablation2] tx %q: %d candidates × %d runs = %d calls",
			tx.hash, len(candidates), runsPerCandidate, len(candidates)*runsPerCandidate)

		categoryCountsByCandidate := make([]map[string]int, len(candidates))
		for i := range categoryCountsByCandidate {
			categoryCountsByCandidate[i] = make(map[string]int)
		}

		for run := 1; run <= runsPerCandidate; run++ {
			// Shuffle call order so temporal effects are distributed randomly
			// across candidate types — in particular, the bad candidate (7)
			// does not always land last in the sequence.
			order := rand.Perm(len(candidates))
			for _, i := range order {
				cooldown()
				alias := fmt.Sprintf("candidate-%d", i+1)
				cat, ok := invokeSingleCandidateJudge(t, "ablation2", run, judgeClient, recorder,
					tx.hash, tx.prompt, alias, candidates[i].label, candidates[i].answer)
				if ok {
					categoryCountsByCandidate[i][cat]++
				}
			}
		}

		// Summary for correct candidates (1–6).
		correctAllRuns := 0
		for i := 0; i < correctCandidates; i++ {
			if categoryCountsByCandidate[i]["CORRECT"] == runsPerCandidate {
				correctAllRuns++
			}
		}

		// Summary for the bad candidate (7).
		badCounts := categoryCountsByCandidate[badCandidateIdx]
		t.Logf("[ablation2] tx %q: correct candidates (1–6): %d/%d CORRECT in all %d runs",
			tx.hash, correctAllRuns, correctCandidates, runsPerCandidate)
		t.Logf("[ablation2] tx %q: bad candidate (7) categories across %d runs: %v",
			tx.hash, runsPerCandidate, badCounts)
		t.Logf("[ablation2] tx %q: batch baseline (Group B diverse round 31): 2–4/7 CORRECT per tx",
			tx.hash)
	}
}
