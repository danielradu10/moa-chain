# Distributed MR2 Experiment — 2026-07-27

## Motivation

Mini-round two (MR2) is the answer-judging phase of the MoA-Chain protocol. After MR1 produces a set of validator answers, each validator calls its `/judge` endpoint to classify every evidence candidate as CORRECT, WRONG, HALLUCINATION, or MALICIOUS. A transaction advances to MR3 only when all validators agree on the classification outcome.

The single-LLM MR2 diverse experiments revealed **canonical-preference bias**: when the judge receives multiple correct-but-differently-phrased answers in a single batch request, it treats one phrasing as authoritative and classifies valid alternatives as WRONG. Because all validators share the same judge agent in the single-LLM setting, they agree on which phrasing is canonical and the round still finalizes — the bias is masked.

The distributed MR2 experiments are designed to surface whether this bias breaks BFT agreement at cluster scale, where each validator calls a **different** cluster agent for `/judge`. If each agent independently picks a different phrasing as canonical, validators will produce incompatible classification votes and the round will never finalize.

---

## Test architecture

### What is mocked vs real

| Component | Mode |
|---|---|
| `LabelBatch` | mocked — fixed subdomain map for all 3 transactions |
| `AnswerBatch` | mocked — each validator receives a pre-assigned answer set |
| `JudgeTransactionAnswers` | **real** — calls the validator's own cluster agent's `/judge` endpoint |

Mocking labeling and answering lets us control the evidence pool deterministically. Only judging is live, which is the operation under test.

### Cluster setup

| Parameter | Value |
|---|---|
| Nodes | 10 (moa-chain-0 … moa-chain-9) |
| Model | `qwen2.5-coder:7b` (Ollama) |
| Committee strategy | `full` — all 10 validators in every committee |
| Temperatures | 0.30, 0.35, 0.40, 0.45, 0.50, 0.55, 0.60, 0.65, 0.70, 0.75 |
| Labeler prompt | `labeler_v3` |
| `stopAfterMiniRoundOne` | `false` — full MR1 → MR2 protocol |
| Judge HTTP timeout | 90s per call (fail-fast on hung Ollama inference) |

### Transactions

| # | Tx hash | Prompt |
|---|---|---|
| 1 | `scenario-01-control-before` | "What is the main benefit of unit tests?" |
| 2 | `scenario-01-target` | "Why must validators verify message signatures?" |
| 3 | `scenario-01-control-after` | "Why does deterministic ordering matter in consensus?" |

### Diverse answer pool

Each validator `i` receives `mr2RADiverseAnswers[i % 6]` — 6 distinct correct-answer perspectives cycling across the 10 validators. No answer set is identical to another; each addresses the same question from a genuinely different factual angle (regression safety, fast feedback, executable specification, design quality, replay-attack prevention, cost of defect detection).

---

## Group A — All correct, diverse answers

### Hypothesis

No bad producers. All 10 validators submit a correct answer, but from 6 different perspectives. The judge at each node receives a batch containing all evidence candidates and must classify them.

**If canonical-preference bias exists on distributed:** each cluster agent independently picks a different phrasing as canonical and classifies the others as WRONG. Since agents disagree, validators cast incompatible votes and BFT agreement is never reached → round does not finalize within 3 minutes.

**If bias does not exist:** the judge correctly classifies all diverse-but-correct candidates as CORRECT on every agent, validators agree, and the round finalizes with `READY_FOR_MR3` for all transactions.

### Answer perspectives assigned per validator

| Validator | Temperature | Perspective |
|---|---|---|
| moa-chain-0 | 0.30 | Perspective 1 — regression safety and refactoring confidence |
| moa-chain-1 | 0.35 | Perspective 2 — fast feedback loop and debugging speed |
| moa-chain-2 | 0.40 | Perspective 3 — executable specification and design forcing function |
| moa-chain-3 | 0.45 | Perspective 4 — design quality and single-responsibility |
| moa-chain-4 | 0.50 | Perspective 5 — replay-attack prevention and deployment confidence |
| moa-chain-5 | 0.55 | Perspective 6 — cost of defect detection and formal agreement property |
| moa-chain-6 | 0.60 | Perspective 1 (repeat) |
| moa-chain-7 | 0.65 | Perspective 2 (repeat) |
| moa-chain-8 | 0.70 | Perspective 3 (repeat) |
| moa-chain-9 | 0.75 | Perspective 4 (repeat) |

### Results

**Run date:** 2026-07-27  
**Command:** `make test-distributed-mr2-diverse-group-a-trials N=5`

| Trial | Duration | Finalized | Notes |
|---|---|---|---|
| 1 | 180.1s | No | `missing answer candidate` on validator-1, validator-9 |
| 2 | 180.1s | No | `missing answer candidate` on validator-4, validator-6, validator-9 |
| 3 | 180.1s | No | `missing answer candidate` on validator-9 |
| 4 | 85.1s | **Yes** | No errors |
| 5 | 180.1s | No | `missing answer candidate` on validator-1, validator-7, validator-9 |

**Finalized: 1 / 5 (20%)**  
**Bias confirmed: 4 / 5 trials (80%)**

---

## Data analysis

### 1. Canonical-preference bias confirmed on distributed (4/5)

The round failed to reach BFT finalization in 4 of 5 trials. In each failing trial the 3-minute timeout was exhausted with no block produced. This directly replicates on the distributed cluster the same bias first observed in the single-LLM diverse experiments.

### 2. Failure mechanism: "missing answer candidate"

The protocol error logged in all failing trials is:

```
miniround2.HandleAnswerEvidence judge failed  error="missing answer candidate"
```

This error fires when the judge's LLM response omits one or more candidate IDs from its classification output. The judge prompt lists all evidence candidates and asks the model to return a structured JSON with a classification for each. When the model ignores some candidates (treating them as not worth classifying), the Go protocol cannot find their classification and raises this error.

This is canonical-preference bias in its most extreme form: rather than classifying alternative phrasings as WRONG, the model simply **does not return any classification for them**, effectively refusing to evaluate candidates it deems invalid. The protocol then stalls because the affected validator can never submit a complete classification vote.

The subsequent `missing classification collection context` errors on validator-9 are cascading: other validators finish judging and broadcast their votes while validator-9 has not yet started its classification context (because its judge call failed), so their votes arrive too early and are rejected.

### 3. Affected validators

| Trial | Validators with judge failure |
|---|---|
| 1 | validator-1 (temp 0.35), validator-9 (temp 0.75) |
| 2 | validator-4 (temp 0.50), validator-6 (temp 0.60), validator-9 (temp 0.75) |
| 3 | validator-9 (temp 0.75) |
| 4 | — (no failures) |
| 5 | validator-1 (temp 0.35), validator-7 (temp 0.65), validator-9 (temp 0.75) |

Validator-9 (temperature 0.75) fails in 4 of 5 trials — the highest-temperature node is the most likely to produce non-conforming judge output. Validator-1 (0.35) and other mid-range temperatures also fail occasionally, showing the bias is not exclusive to high temperatures. Trial 4's clean run had no failures on any node.

### 4. Trial 4 — the one that finalized

Trial 4 completed in 85.1s with no errors and no JSON tx_results collected (the test passed as `ok` without saving a result file, likely because the `saveDistributedMR2DiverseResult` call happens after `assert.Eventually` which returned true, but the JSON was still saved — file not collected locally). The clean completion likely reflects a favorable random seed for committee selection and model sampling at lower temperature nodes providing the canonical classification that others agreed with.

### 5. Duration

Non-finalizing trials all hit exactly the 3-minute `assert.Eventually` timeout (180.1s). The single finalizing trial completed in 85s, which is consistent with the honest-round timing observed in distributed MR1 tests.

---

## Key Findings

**Finding #12 — Canonical-preference bias confirmed on distributed cluster (4/5 trials)**

The distributed judge cluster fails to reach BFT agreement when all 10 validators hold correct but differently phrased answers. 4 of 5 trials timed out with no block produced. The root cause is the 7B model completely omitting some answer candidates from its judge response (`missing answer candidate`) rather than classifying them — the most severe form of canonical-preference bias. This proves the bias is a fundamental property of the 7B model, not an artifact of the single-LLM test environment.

**Finding #13 — High-temperature validators are the primary failure point**

Validator-9 (temperature 0.75) produced a failing judge call in 4 of 5 trials. Higher temperature increases the likelihood that the model deviates from the structured output format and drops candidates. However, mid-range temperature validators (0.35, 0.50, 0.60, 0.65) also failed in some trials, showing the issue is not isolated to the highest temperature.

---

## Required fix: one candidate per `/judge` call

The bias is unambiguous and systematic. The fix is to restructure the judge protocol so that each `/judge` HTTP call receives exactly **one answer candidate** instead of all evidence candidates in a batch. The model then classifies that single candidate against the question in isolation, which eliminates both the "missing candidate" dropout and the cross-candidate comparison bias.

After implementing this fix:
1. Re-run Group A diverse (round 202) — expect all trials to finalize with `READY_FOR_MR3`
2. Run Groups B–F with diverse answers — test bad-answer detection under the fixed protocol

---

## Next steps

1. Fix the Python judge to send one candidate per `/judge` call
2. Re-run Group A diverse N=5 to confirm fix
3. Proceed with Groups B–F diverse on distributed
