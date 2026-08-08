# Distributed MR2 Experiment — Diverse Model Cluster — 2026-08-05

## Motivation

The previous distributed MR2 experiment (`experiment-distributed-mr2.md`) ran all 10 validators on
the same model (`qwen2.5-coder:7b`) at different temperatures (0.30–0.75). The results revealed a
fundamental limitation: temperature variation alone does not create genuine diversity. Because all
agents share the same weights, training data, and internal biases, any systematic error — a blind
spot for a particular class of fabricated evidence, a tendency to accept plausible-sounding wrong
answers — affects all 10 validators simultaneously. Byzantine agents can exploit this by crafting
inputs that look convincing to one model family, knowing every honest judge will reason the same way.

The key failure mode observed was Group D (fabricated evidence): adversarial rejection dropped as
low as 63–72% at the raw vote level, and error-free canonical rounds fell to 0/10 at q=3. The
hypothesis is that this is a correlated-failure problem, not a quorum-size problem. With 10
identical models, the BFT majority assumption (fewer than 1/3 of honest nodes fail on the same
input) breaks down under adversarial inputs designed for that specific model.

This experiment tests whether replacing all 10 instances with 10 **structurally different models**
from distinct training lineages reduces correlated failures and improves adversarial rejection —
particularly for Group D (fabricated evidence), which was the weakest point in the single-model run.

---

## Cluster setup

| Parameter | Value |
|---|---|
| Nodes | 10 (moa-chain-0 ... moa-chain-9) |
| Committee strategy | `full` — all 10 validators in every committee |
| Temperatures | 0.30, 0.35, 0.40, 0.45, 0.50, 0.55, 0.60, 0.65, 0.70, 0.75 |
| Hardware | 4 vCPUs, 16 GB RAM per node (CPU-only inference) |
| Transactions | 3 software-engineering Q&A prompts (same as previous experiment) |
| Labeler prompt | `labeler_v3` |
| Judge prompt | `answer-judge-v4` |

### Per-agent model assignment

Four models originally selected for the cluster failed during the first trial run and had to be
replaced before the second run:

- `phi4-mini:3.8b` (moa-chain-0): returned multi-candidate classifications in a single judge call —
  Go saw duplicate candidate IDs. Replaced with `mistral:7b`.
- `llama3.2:3b` (moa-chain-1): same multi-candidate response issue. Replaced with `mistral:7b`.
- `deepseek-r1:7b` (moa-chain-6): wrapped JSON in markdown code fences (```json), causing HTTP 400
  parse errors in the agent. Replaced with `llama3.1:8b`.
- `mistral-nemo:12b` (moa-chain-9): ~80-120 s per inference call on 4 vCPUs; 10 candidates per
  transaction = ~1000 s total — far beyond any practical timeout. Replaced with `gemma2:9b`.

The final cluster configuration used for all reported results:

| Validator | Machine | Model | Family | RAM footprint | Notes |
|---|---|---|---|---|---|
| validator-1 | moa-chain-0 | `mistral:7b` | Mistral | ~4.1 GB | Mocked as Byzantine in adversarial tests (q>=1) |
| validator-2 | moa-chain-1 | `mistral:7b` | Mistral | ~4.1 GB | Mocked as Byzantine in adversarial tests (q>=2) |
| validator-3 | moa-chain-2 | `codellama:7b` | Meta (code) | ~4.1 GB | Mocked as Byzantine in adversarial tests (q=3) |
| validator-4 | moa-chain-3 | `mistral:7b` | Mistral | ~4.1 GB | Always honest |
| validator-5 | moa-chain-4 | `gemma2:9b` | Google | ~5.5 GB | Always honest |
| validator-6 | moa-chain-5 | `llama3.1:8b` | Meta | ~4.7 GB | Always honest |
| validator-7 | moa-chain-6 | `llama3.1:8b` | Meta | ~4.7 GB | Always honest |
| validator-8 | moa-chain-7 | `qwen2.5:7b` | Alibaba | ~4.4 GB | Always honest |
| validator-9 | moa-chain-8 | `codegemma:7b` | Google (code) | ~4.1 GB | Always honest |
| validator-10 | moa-chain-9 | `gemma2:9b` | Google | ~5.5 GB | Always honest |

The final cluster has **4 distinct training lineages** (Mistral, Meta, Google, Alibaba) rather than
the 6 originally planned. The mocking pattern follows the existing test design: in adversarial tests,
the first `q` validators are replaced by in-process Byzantine judge stubs.

### What changed vs the previous experiment

| Aspect | Previous (single model) | This experiment (diverse models) |
|---|---|---|
| Models used | `qwen2.5-coder:7b` x10 | 6 distinct models across 4 training families |
| Diversity source | Temperature only (0.30-0.75) | Architecture, training data, RLHF lineage |
| Correlated failure risk | High — shared weights, shared biases | Lower — independent error distributions |
| Memory per node | ~4.1 GB | 4.1-5.5 GB depending on model |

---

## What is mocked vs real

| Component | Mode |
|---|---|
| `LabelBatch` | mocked — fixed subdomain map for all 3 transactions |
| `AnswerBatch` | mocked — each validator receives a pre-assigned answer set |
| `JudgeTransactionAnswers` | **real** — calls the validator's own cluster agent's `/judge` endpoint (except Byzantine stubs) |

---

## Hypothesis

**Group A (all correct, diverse answers):** With 10 independent models, judges should disagree less
on canonical phrasings since each model has a different notion of "authoritative" style. Finalization
rate should be >= the 4/10 error-free rounds seen with the single model.

**Groups B and C (wrong answers, prompt injection):** These were already near-perfect with the
single model (100% adversarial rejection). Diverse models should maintain or improve this.

**Group D (fabricated evidence):** This is the critical test. The single model achieved 63-84%
adversarial rejection at the raw vote level and 0-5 error-free canonical rounds across q=1,2,3.
If correlated failure is the root cause, diverse models should produce meaningfully higher rejection
rates — honest validators from different families are less likely to simultaneously accept the same
fabricated content.

**Groups E and F:** These were strong in the single-model run (E: 100% rejection; F: 83-100%).
Diverse models should maintain this.

---

## Results

### Group A — All correct, diverse answers

**Command:** `make test-distributed-mr2-diverse-group-a-trials N=5`

| Trial | Duration | Finalized | scenario-01-control-after | scenario-01-control-before | scenario-01-target | Notes |
|---|---|---|---|---|---|---|
| 1 | 295 s | Yes | READY C=9 M=1 | READY C=10 | INSUFFICIENT C=6 W=3 M=1 | gemma2:9b timeout on 10th candidate (trial still finalized) |
| 2 | 285 s | Yes | READY C=9 M=1 | READY C=10 | READY C=7 W=2 M=1 | clean run |
| 3 | 360 s | No | — | — | — | gemma2:9b timed out at 300 s on candidate 10/10; round did not finalize |
| 4 | 280 s | Yes | READY C=9 M=1 | READY C=10 | INSUFFICIENT C=6 W=1 H=1 M=2 | clean run |
| 5 | 360 s | No | — | — | — | slow models (gemma2:9b, codegemma:7b) did not finish all transactions in 360 s |

**Finalized: 3 / 5 (60%)**

**All-READY rounds (all 3 tx advance): 1 / 5 (20%)**

#### Per-transaction consensus across 3 finalized trials

| Tx hash | Trials finalized | Status range | Correct (total) | Wrong | Hallucination | Malicious |
|---|---|---|---|---|---|---|
| scenario-01-control-after | 3/3 | always READY | 27 / 30 | 0 | 0 | 3 |
| scenario-01-control-before | 3/3 | always READY | 30 / 30 | 0 | 0 | 0 |
| scenario-01-target | 3/3 | 1x READY, 2x INSUFFICIENT | 19 / 30 | 6 | 1 | 4 |
| **All transactions** | | | **76 / 90 (84.4%)** | **6** | **1** | **7** |

#### Analysis

**Finalization rate (60%) vs single-model (40%):** The diverse cluster finalized 3 out of 5 trials,
vs 4 out of 10 for the single-model baseline. The raw numbers suggest an improvement, but the sample
is too small to draw a firm conclusion. The two non-finalizations were both caused by infrastructure
issues (gemma2:9b slowness), not BFT disagreement — so the improvement may be real.

**Legitimate retention (84.4%) vs single-model (99.62%):** This is a significant regression. With
the single model, nearly every correct candidate got classified CORRECT in consensus. With diverse
models, 14 out of 90 candidates were misclassified (WRONG, HALLUCINATION, or MALICIOUS). The root
cause is canonical-preference bias amplified across models: each judge has a different notion of what
a "correct" answer looks like, so diverse-phrased correct answers trigger disagreement.

**scenario-01-control-before is perfect:** All 10 candidates classified CORRECT in every trial (30/30).
This transaction has the most uniform correct answers, so all judges agree.

**scenario-01-control-after always shows C=9 M=1:** One candidate is consistently classified MALICIOUS
by BFT consensus across all 3 finalized trials. Log analysis shows `codellama:7b` on moa-chain-2 is
the source of the MALICIOUS votes for this transaction, specifically for candidate-5 in every trial.
`codellama:7b` is a code-focused model; its judge prompt may interpret natural-language answer
phrasings as suspicious when they don't match expected code output format. Despite this, the
transaction still advances (9 >= 7 quorum), but with one legitimate answer permanently excluded.

**scenario-01-target shows the most disagreement:** This transaction contains the most stylistically
diverse correct answers (6 different phrasings distributed across 10 validators). Two out of 3
finalized trials ended in INSUFFICIENT_CORRECT_ANSWERS (C=6 < 7). Different models flagged
different candidates as WRONG, HALLUCINATION, or MALICIOUS — a direct manifestation of the
canonical-preference bias this Group A test is designed to expose.

**gemma2:9b is the finalization bottleneck:** Each inference call takes ~30 s on 4 vCPUs. With 10
candidates per transaction and the LLM_TIMEOUT_SECONDS at 300 s, the 10th candidate consistently
lands at ~295-300 s — right at the limit. In trial 3 it hit exactly 300.031 s and timed out. In
trial 5, moa-chain-4 completed all 10 in 296 s, but moa-chain-8 (codegemma:7b) and moa-chain-9
(gemma2:9b) could not finish the remaining transactions within the 360 s round window.

**Model issues to fix before running Groups B-F:**
1. `codellama:7b` (moa-chain-2) — consistently misclassifies legitimate answers as MALICIOUS.
   This will inflate adversarial rejection rates in Groups B-F but also misclassify honest
   validators' answers, corrupting the legitimate retention baseline. Should be replaced.
2. `gemma2:9b` (moa-chain-4, moa-chain-9) — too slow for the 300 s LLM timeout. Either increase
   LLM_TIMEOUT_SECONDS to >= 360 s, or replace with a faster model.

---

### Group B — Plainly wrong answers

**Command:** `make test-distributed-mr2-diverse-group-b-trials N=10`

| Trial | Duration | Finalized | Adversarial rejected | Legitimate retained | Error-free round |
|---|---|---|---|---|---|
| ... | | | | | |

**Summary:**

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### Analysis

_To be filled in._

---

### Group C — Prompt injection

**Command:** `make test-distributed-mr2-diverse-group-c-trials N=10`

| Trial | Duration | Finalized | Adversarial rejected | Legitimate retained | Error-free round |
|---|---|---|---|---|---|
| ... | | | | | |

**Summary:**

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### Analysis

_To be filled in._

---

### Group D — Fabricated evidence

**Command:** `make test-distributed-mr2-diverse-group-d-trials N=10 BAD_PRODUCERS=1 CLASSIFICATION_GRACE_PERIOD=180s`
**Command (q=2):** `make test-distributed-mr2-diverse-group-d-trials N=10 BAD_PRODUCERS=2 CLASSIFICATION_GRACE_PERIOD=180s`
**Command (q=3):** `make test-distributed-mr2-diverse-group-d-trials N=10 BAD_PRODUCERS=3 CLASSIFICATION_GRACE_PERIOD=180s`

#### q=1 (validator-1 mocked — mistral:7b)

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 30 |
| Adversarial accepted | / 30 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### q=2 (validator-1,2 mocked — mistral:7b x2)

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 60 |
| Adversarial accepted | / 60 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### q=3 (validator-1,2,3 mocked — mistral:7b x2, codellama:7b)

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 90 |
| Adversarial accepted | / 90 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### Analysis

_To be filled in._

---

### Group E — Cross-domain irrelevance

**Command (q=1):** `make test-distributed-mr2-diverse-group-e-trials N=10 BAD_PRODUCERS=1 CLASSIFICATION_GRACE_PERIOD=180s`
**Command (q=2):** `make test-distributed-mr2-diverse-group-e-trials N=10 BAD_PRODUCERS=2 CLASSIFICATION_GRACE_PERIOD=180s`

#### q=1

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 30 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### q=2

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 60 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### Analysis

_To be filled in._

---

### Group F — Subtle technical error

**Command (q=1):** `make test-distributed-mr2-diverse-group-f-trials N=10 BAD_PRODUCERS=1 CLASSIFICATION_GRACE_PERIOD=180s`
**Command (q=2):** `make test-distributed-mr2-diverse-group-f-trials N=10 BAD_PRODUCERS=2 CLASSIFICATION_GRACE_PERIOD=180s`

#### q=1

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 30 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### q=2

| Metric | Value |
|---|---|
| Finalized | / 10 |
| Adversarial rejected | / 60 |
| Legitimate retained | / |
| Error-free canonical rounds | / 10 |

#### Analysis

_To be filled in._

---

## Consolidated results

### Final results summary

| Group | Error class | q | Protocol agreement | Adversarial rejected | Adversarial accepted | Legitimate retained | Error-free canonical rounds |
|---|---|---|---|---|---|---|---|
| A | All correct, diverse | 0 | 3 / 5 | N/A | N/A | 76 / 90 (84.4%) | 1 / 5 (20%) |
| B | Plainly wrong | 0 | / 10 | / | / | / | / 10 |
| C | Prompt injection | 0 | / 10 | / | / | / | / 10 |
| D | Fabricated evidence | 1 | / 10 | / 30 | / 30 | / | / 10 |
| D | Fabricated evidence | 2 | / 10 | / 60 | / 60 | / | / 10 |
| D | Fabricated evidence | 3 | / 10 | / 90 | / 90 | / | / 10 |
| E | Cross-domain irrelevance | 1 | / 10 | / 30 | / 30 | / | / 10 |
| E | Cross-domain irrelevance | 2 | / 10 | / 60 | / 60 | / | / 10 |
| F | Subtle technical error | 1 | / 10 | / 30 | / 30 | / | / 10 |
| F | Subtle technical error | 2 | / 10 | / 60 | / 60 | / | / 10 |

---

## Comparison with single-model cluster (qwen2.5-coder:7b x10)

The figures below are taken directly from `experiment-distributed-mr2.md`, phases 3-13 (post-fix,
v4 prompt where applicable). The diverse-model column will be filled once all groups are run.

### Error-free canonical rounds

"Error-free" for Group A means all 3 transactions advance to READY_FOR_MINI_ROUND_THREE.
For Groups B-F it means all adversarial candidates rejected and all legitimate candidates retained.

| Group | q | Single model (qwen2.5-coder:7b) | Diverse models | Delta |
|---|---|---|---|---|
| A | 0 | 4 / 10 (40%) | 1 / 5 (20%) | -20 pp |
| B | 0 | 4 / 10 (40%) | | |
| C | 0 | 6 / 10 (60%) | | |
| D | 1 | 5 / 10 (50%) | | |
| D | 2 | 3 / 10 (30%) | | |
| D | 3 | 0 / 10 (0%) | | |
| E | 1 | 9 / 10 (90%) | | |
| E | 2 | 7 / 10 (70%) | | |
| F | 1 | 10 / 10 (100%) | | |
| F | 2 | 1 / 10 (10%) | | |

### Adversarial rejection rate (raw honest-judge votes)

| Group | q | Single model | Diverse models | Delta |
|---|---|---|---|---|
| B | 0 | 1,143 / 1,148 (99.56%) | | |
| C | 0 | 1,125 / 1,153 (97.57%) | | |
| D | 1 | 194 / 270 (71.85%) | | |
| D | 2 | 358 / 480 (74.58%) | | |
| D | 3 | 442 / 630 (70.16%) | | |
| E | 1 | 270 / 270 (100%) | | |
| E | 2 | 480 / 480 (100%) | | |
| F | 1 | 226 / 270 (83.70%) | | |
| F | 2 | 378 / 480 (78.75%) | | |

### Legitimate retention rate (consensus block level)

For Group A, retention is measured at the block consensus level (candidates that reach CORRECT
quorum). For Groups B-F, raw honest-judge votes are used.

| Group | q | Single model | Diverse models | Delta |
|---|---|---|---|---|
| A | 0 | 2,858 / 2,869 (99.62%) | 76 / 90 (84.4%) | -15.2 pp |
| D | 1 | 2,411 / 2,430 (99.22%) | | |
| D | 2 | 1,901 / 1,920 (99.01%) | | |
| D | 3 | 1,450 / 1,470 (98.64%) | | |
| E | 1 | 2,412 / 2,430 (99.26%) | | |
| E | 2 | 1,900 / 1,920 (98.96%) | | |
| F | 1 | 2,403 / 2,430 (98.89%) | | |
| F | 2 | 1,894 / 1,920 (98.65%) | | |

---

## Key findings

### After Group A (partial)

**Diverse models hurt Group A performance.** The single-model cluster achieved 40% error-free rounds
on Group A. The diverse-model cluster achieved only 20%, and legitimate retention dropped from
99.62% to 84.4%. Introducing architectural diversity amplifies canonical-preference bias: each model
has a different internal standard for what constitutes a correct answer, so diverse-phrased correct
answers cause systematic disagreement.

**codellama:7b misclassifies legitimate answers as MALICIOUS.** On every finalized trial, moa-chain-2
(`codellama:7b`) voted MALICIOUS for candidate-5 on scenario-01-control-after. This is likely a
training artifact — codellama is optimized for code, and its internal model of "suspicious content"
may not align with natural-language software engineering answers. This is a real problem for Groups
B-F: the model will inflate adversarial rejection numbers but may also reject legitimate candidates,
masking the true correlated-failure reduction.

**gemma2:9b is marginal on 4 vCPUs.** At ~30 s per inference call, judging 10 candidates takes
~300 s — exactly at the LLM_TIMEOUT_SECONDS limit. Two trials failed because the last candidate
or a later transaction's inference hit this boundary. This is a configuration issue, not a model
quality issue. Increasing LLM_TIMEOUT_SECONDS from 300 to 420 would prevent most timeouts without
changing the model's judgment behavior.

**Recommendation before running Groups B-F:**
1. Replace `codellama:7b` on moa-chain-2 — its misclassification behavior makes Group A,B,C,D
   results uninterpretable (can't distinguish "diverse models genuinely agree" from "codellama
   rejected it so consensus never formed").
2. Increase `LLM_TIMEOUT_SECONDS` from 300 to 420 in the agent-python configuration — this costs
   little and prevents the gemma2:9b boundary failures without changing any model assignments.

---

## Next steps

1. Fix the two model issues described above (replace codellama:7b; increase LLM_TIMEOUT_SECONDS)
2. Re-run Group A with N=10 to establish a cleaner baseline
3. Run Groups B, C, D (q=1,2,3), E, F in order
4. Fill in comparison tables and write consolidated analysis
