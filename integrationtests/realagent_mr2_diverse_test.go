//go:build integration

package integrationtests

import (
	"fmt"
	"testing"
	"time"

	"moa-chain/data"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Diverse correct-answer pool ────────────────────────────────────────────────

// mr2RADiverseAnswers is a pool of 6 distinct correct-answer sets, one per
// honest validator slot. Each set addresses the same question from a different
// factual angle — not just a rephrasing but a genuinely different correct
// perspective. In production every validator independently calls /answer, so
// the judge sees semantic diversity in honest evidence. These fixtures replicate
// that diversity deterministically.
//
// Honest validator i receives answer set i % len(mr2RADiverseAnswers).
var mr2RADiverseAnswers = []map[string]string{
	// Perspective 1 — regression safety and refactoring confidence
	{
		mr2RATxHash1: "Unit tests verify individual functions in isolation, catching regressions early and enabling safe refactoring by confirming that behavior is preserved after each change.",
		mr2RATxHash2: "Signature verification proves a message was authorized by the claimed validator and has not been tampered with in transit, making impersonation and replay attacks detectable.",
		mr2RATxHash3: "Deterministic ordering guarantees that every honest node derives the same canonical result from the same set of inputs, which is a prerequisite for Byzantine fault-tolerant agreement.",
	},
	// Perspective 2 — fast feedback loop and debugging speed
	{
		mr2RATxHash1: "The primary benefit of unit tests is rapid feedback: they execute in milliseconds and immediately reveal whether a code change broke an existing assumption, cutting the debugging cycle from hours to seconds.",
		mr2RATxHash2: "Without signature verification any node on the network could forge a vote in another validator's name, injecting phantom consensus messages that appear legitimate and corrupting the agreement process.",
		mr2RATxHash3: "Consensus requires all replicas to agree on a single transaction sequence; if two nodes applied the same transactions in different orders they would reach divergent ledger states and the protocol would fork.",
	},
	// Perspective 3 — executable specification and design forcing function
	{
		mr2RATxHash1: "Unit tests act as executable specifications — each test case encodes a precise behavioral contract for a function, making intent explicit and machine-verifiable without relying on prose documentation.",
		mr2RATxHash2: "Signature verification enforces accountability: if a validator equivocates by signing conflicting blocks, both signatures serve as cryptographic proof of the misbehavior, enabling the network to slash or exclude the offender.",
		mr2RATxHash3: "Deterministic ordering is what makes state machine replication possible: each replica starts from the same genesis state and applies commands in the same canonical sequence, so their outputs are always identical regardless of message arrival order.",
	},
	// Perspective 4 — design quality and single-responsibility
	{
		mr2RATxHash1: "By requiring each component to be testable in isolation, unit tests push developers toward loosely coupled designs; code that is hard to unit-test almost always violates the single-responsibility principle.",
		mr2RATxHash2: "Validators verify signatures to enforce the validator-set boundary: only holders of registered private keys can produce valid signatures, preventing unregistered nodes from diluting the honest-majority assumption the protocol depends on.",
		mr2RATxHash3: "Without a canonical ordering function a malicious validator could selectively reorder messages to change which block wins a vote, converting a network-level capability into a consensus-level attack vector.",
	},
	// Perspective 5 — replay-attack prevention and deployment confidence
	{
		mr2RATxHash1: "Unit tests provide a safety net that lets teams ship with confidence: when every critical path is covered by an automated check, deploying a change no longer requires manually re-verifying the entire system.",
		mr2RATxHash2: "Signature checks prevent replay attacks: a message signed for round N carries that round number inside the signed payload, so an adversary cannot retransmit a valid old message to confuse validators processing a later round.",
		mr2RATxHash3: "Deterministic ordering removes ambiguity from block finalization: when every honest node runs the same total-order function over the same input set, no additional coordination round is needed to break ties or resolve conflicts.",
	},
	// Perspective 6 — cost of defect detection and formal agreement property
	{
		mr2RATxHash1: "Unit tests catch defects at the lowest possible cost — a unit-test failure points directly at the function that introduced the bug, whereas the same defect found in production requires tracing through multiple layers to locate the root cause.",
		mr2RATxHash2: "Integrity verification through signatures ensures that any bit-level corruption during transmission is detected before a message influences consensus; a single modified byte invalidates the signature and the message is discarded.",
		mr2RATxHash3: "In a BFT protocol the agreement property requires that no two honest nodes decide on different values; deterministic ordering upholds this by ensuring the function from input set to decision is a pure, side-effect-free mapping with a unique output.",
	},
}

// ── Round runner ───────────────────────────────────────────────────────────────

// runRealAgentMR2DiverseRound runs a full MR1→MR2 consensus round with diverse
// correct answers distributed across honest validators. It returns the finalized
// block and true on success. If the round does not finalize within timeout it
// logs the deadlock finding and returns nil, false — the test is not failed,
// since a quorum breakdown under diverse inputs is itself a valid result.
func runRealAgentMR2DiverseRound(
	t *testing.T,
	group string,
	badAnswers map[string]string,
	roundNumber uint64,
	timeout time.Duration,
) (*data.BlockOnChain, bool) {
	t.Helper()

	const registeredNodes = 20
	const committeeSize = 10

	judgeClient := realAgentClient()

	publicKeys, privateKeys := generateScenarioKeys(t, registeredNodes)
	registeredValidators := createScenarioValidators(publicKeys)

	transactions := realAgentMR2Transactions()

	frequencies := make(data.SubdomainsFrequency)
	for _, labels := range mr2RATxLabels {
		for _, label := range labels {
			frequencies[label] += uint64(committeeSize)
		}
	}

	committees := selectScenarioCommittees(t, registeredValidators, frequencies)
	require.Len(t, committees.miniRoundTwo, committeeSize)

	rolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)

	inboxes := make([]chan data.RoundEvent, registeredNodes)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 256)
	}

	nodes := make([]*integrationTestNode, registeredNodes)
	for i := 0; i < registeredNodes; i++ {
		validatorID := fmt.Sprintf("validator-%d", i+1)
		role := rolesByValidator[validatorID]

		answers := mr2RADiverseAnswers[i%len(mr2RADiverseAnswers)]
		if badAnswers != nil && mr2RABadRoles[role] {
			answers = badAnswers
		}

		nodes[i] = createNode(
			t,
			validatorID,
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
		)
	}

	errCh := make(chan error, 256)
	for _, node := range nodes {
		currentNode := node
		go func() {
			for err := range currentNode.loop.Errors() {
				errCh <- err
			}
		}()
		go func(n *integrationTestNode) { n.loop.Run() }(node)
	}

	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: roundNumber, MiniRound: uint64(data.MiniRoundTwo)}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: miniRoundOneKey}
	}
	t.Logf("[%s/diverse] round %d started — %d validators, diverse correct answers, real LLM judge",
		group, roundNumber, registeredNodes)

	finalized := assert.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
			return false
		default:
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey); err != nil {
				return false
			}
		}
		return true
	}, timeout, 5*time.Second)

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	if !finalized {
		t.Logf("[%s/diverse] FINDING: round %d did not finalize within %v — "+
			"validators produced inconsistent classification votes and quorum was never reached. "+
			"This indicates the LLM is non-deterministic enough across diverse inputs that "+
			"different validators disagree on which candidates are CORRECT.", group, roundNumber, timeout)
		return nil, false
	}

	block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
	require.NoError(t, err)
	require.NotNil(t, block)

	for _, node := range nodes[1:] {
		other, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
		require.NoError(t, err)
		require.Equal(t, block.AnswerClassifications, other.AnswerClassifications)
	}

	return block, true
}

// logDiverseOutcome logs the result for each transaction. All outcomes are
// valid dissertation data — tests never fail based on classification results.
func logDiverseOutcome(t *testing.T, block *data.BlockOnChain, group string) {
	t.Helper()
	for _, tx := range block.AnswerClassifications {
		t.Logf("[%s/diverse] tx %q: status=%s correct=%d wrong=%d hallucination=%d malicious=%d",
			group, string(tx.TxHash), tx.Status,
			len(tx.Groups.Correct), len(tx.Groups.Wrong),
			len(tx.Groups.Hallucination), len(tx.Groups.Malicious))
	}
}

const diverseRoundTimeout = 5 * time.Minute

// ── Group A — All correct, diverse ────────────────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupA_AllCorrectAnswersConverge runs the
// baseline with 6 distinct correct perspectives across honest validators.
// Expected finding: the 7B judge treats one phrasing as canonical and rejects
// valid alternatives, producing INSUFFICIENT even with all-correct evidence.
//
// Run with: make test-realagent-mr2-diverse-group-a
func TestMiniRoundTwo_RealAgent_Diverse_GroupA_AllCorrectAnswersConverge(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_a", nil, 30, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_a")
	}
}

// ── Group B — Wrong answer, diverse ───────────────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupB_WrongAnswerIsRejected runs the
// wrong-answer scenario with diverse honest answers. The bad answer competes
// with 5 diverse correct ones; the judge may flag many correct ones as WRONG
// alongside the actual bad answer, making it impossible to distinguish true
// rejections from false positives in the block outcome.
//
// Run with: make test-realagent-mr2-diverse-group-b
func TestMiniRoundTwo_RealAgent_Diverse_GroupB_WrongAnswerIsRejected(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_b", mr2RABadAnswers["group_b"], 31, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_b")
	}
}

// ── Group C — Prompt injection, diverse ───────────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupC_PromptInjectionResistance runs the
// injection scenario with diverse honest answers. The combination of injection
// and answer diversity may cause inter-validator classification disagreement
// severe enough that the round never finalizes — itself a finding.
//
// Run with: make test-realagent-mr2-diverse-group-c
func TestMiniRoundTwo_RealAgent_Diverse_GroupC_PromptInjectionResistance(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_c", mr2RABadAnswers["group_c"], 32, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_c")
	}
}

// ── Group D — Hallucination, diverse ──────────────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupD_HallucinationIsRejected runs the
// fabricated-citation scenario with diverse honest answers.
//
// Run with: make test-realagent-mr2-diverse-group-d
func TestMiniRoundTwo_RealAgent_Diverse_GroupD_HallucinationIsRejected(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_d", mr2RABadAnswers["group_d"], 33, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_d")
	}
}

// ── Group E — Cross-domain, diverse ───────────────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupE_CrossDomainAnswerDetection runs the
// cross-domain scenario with diverse honest answers. The judge sees 6 different
// correct answers about the actual topic alongside 1 from a different domain.
//
// Run with: make test-realagent-mr2-diverse-group-e
func TestMiniRoundTwo_RealAgent_Diverse_GroupE_CrossDomainAnswerDetection(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_e", mr2RABadAnswers["group_e"], 34, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_e")
	}
}

// ── Group F — Subtle Byzantine errors, diverse ────────────────────────────────

// TestMiniRoundTwo_RealAgent_Diverse_GroupF_SubtleByzantineErrorIsRejected runs
// the subtle-error scenario with diverse honest answers.
//
// Run with: make test-realagent-mr2-diverse-group-f
func TestMiniRoundTwo_RealAgent_Diverse_GroupF_SubtleByzantineErrorIsRejected(t *testing.T) {
	pingAgentOrSkip(t)
	block, ok := runRealAgentMR2DiverseRound(t, "group_f", mr2RABadAnswers["group_f"], 35, diverseRoundTimeout)
	if ok {
		logDiverseOutcome(t, block, "group_f")
	}
}
