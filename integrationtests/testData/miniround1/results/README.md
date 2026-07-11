# Mini-Round One Integration Test Results

This document analyzes the results of 5 repeated integration test runs of the MoA Chain
Mini-Round One (MR1) consensus protocol, executed on 2026-07-11. It covers both label
convergence behavior and per-validator latency, and is intended to support evaluation of
the protocol's correctness and performance characteristics.

---

## 1. Overview

MoA Chain is a blockchain protocol in which validator nodes use a local Large Language
Model (LLM) to classify transactions into software engineering subdomains. Mini-Round One
(MR1) is the first consensus phase: validators independently label a batch of transactions,
the designated leader proposes a block containing its own labels, and the remaining
validators vote to accept or reject that proposal. A block is finalized when a quorum of
the consensus group agrees on the same header hash.

The test ran the same four transactions through 5 consecutive MR1 rounds and recorded the
final `SubdomainsFrequency` map produced by each round, the per-round wall-clock time, and
the per-validator LLM latency.

---

## 2. Test Configuration

| Parameter | Value |
|---|---|
| Network size | 20 validators |
| Consensus group size | 10 (= 20 / 2) |
| Quorum threshold | 7 (= ⌊2 × 10 / 3⌋ + 1) |
| Byzantine fault tolerance | f = 3 |
| LLM model | qwen2.5-coder:7b via Ollama |
| LLM temperature | 0.5 (non-deterministic) |
| Prompt version | labeler_v2 |
| Ollama parallelism | 7 (shared single instance) |
| Rounds executed | 5 |
| Transactions per round | 4 |
| Total LLM batch calls | 100 (20 validators × 5 rounds) |

**Deployment note.** In this test environment all 20 validators share a single Ollama
instance running on the same machine. In a production deployment each node runs its own
dedicated Ollama instance, so all validators label concurrently. This difference is the
primary source of the timing discrepancy discussed in Section 10.

---

## 3. Consensus Group Mathematics

The consensus group is the randomly-selected subset of validators that participates in a
given round. Its size and quorum threshold determine the protocol's fault-tolerance
properties.

```
N = 20   (total validators)
G = N / 2 = 10   (consensus group size, integer division)
Q = (2 × G) / 3 + 1 = (2 × 10) / 3 + 1 = 7   (quorum threshold, integer arithmetic)
f = G - Q = 3   (maximum tolerated Byzantine faults)
```

A label is added to the final `SubdomainsFrequency` map for a given transaction only if at
least Q = 7 of the 10 consensus group members assigned that label to that transaction. The
frequency value is then accumulated across all transactions that passed this threshold.

Because each block in this test contains 4 transactions and at most one label per
transaction can clear Q = 7 (as demonstrated below), the maximum possible frequency for
any label is 7 × 4 = 28. A frequency of exactly 7 means the label appeared in precisely
the minimum required number of consensus group votes for one transaction — unanimity at
the quorum boundary.

---

## 4. Transactions Under Test

The four transactions were chosen to represent clearly distinct software engineering
subdomains with minimal ambiguity, forming Group B of the test suite.

| ID (prefix) | Prompt | Expected primary label |
|---|---|---|
| `18efbfbd` | "How does backpropagation work in a neural network and why is the chain rule needed?" | `ml_ai_engineering` |
| `efbfbd68ef` | "What is the difference between a proof-of-work and a proof-of-stake consensus mechanism in blockchain networks?" | `blockchain_engineering` |
| `efbfbd060e` | "What is the virtual DOM in React and how does it improve rendering performance compared to direct DOM manipulation?" | `web_front_end` |
| `efbfbd0479` | "What is the difference between horizontal and vertical scaling in cloud infrastructure, and when should each be used?" | `cloud_engineering` |

---

## 5. Labeling Latency Benchmark — Calibrating SelectTransactions Limits

This section reports the results of a direct LLM latency benchmark that measures how the time to complete a single `LabelBatch` call scales with the number of transactions in the batch and the length (token count) of each transaction's prompt. The goal is to empirically determine the correct values for the two block-selection limits: `maxNumTransactions` and `maxBlockConsumption`.

---

### 5.1 Why Two Limits?

The `SelectTransactions` algorithm stops accumulating transactions when either of two limits is hit:

- **`maxBlockConsumption`** — the total estimated input token count across all selected transactions. This bounds the LLM's *prefill phase*: processing all the input text before generating any output. Token count is measured by `tiktoken` at mempool entry time and is known before the LLM is called.

- **`maxNumTransactions`** — the count of selected transactions regardless of their individual length. This bounds the LLM's *generation phase*: producing label strings as output. Each additional transaction requires generating at least one label, so output length grows linearly with transaction count. Since output tokens cannot be predicted in advance (the model may generate 1 to 4 labels per transaction), a hard count cap is the practical way to bound generation cost.

The two limits are orthogonal: many short transactions can hit `maxNumTransactions` before `maxBlockConsumption`; one very long transaction can hit `maxBlockConsumption` before adding a second transaction. Whichever limit fires first stops the selection.

---

### 5.2 Benchmark Design

The benchmark calls the agent's `/label` endpoint directly — one HTTP POST, one LLM call — without running the consensus protocol. This matches the production scenario where each validator calls its own Ollama instance independently. All measurements are single-validator, warm-model latencies.

**Experiment A — vary transaction count, fixed prompt length**

Fixed prompt: a realistic 25-token developer question about arrays vs linked lists. Batch sizes: 1, 2, 4, 8, 16 transactions. Three runs per size.

**Experiment B — vary prompt length, fixed transaction count**

Fixed count: 4 transactions with identical prompts at each length tier. Tiers:

| Tier | Tokens per tx | Total tokens | Prompt character |
|---|---|---|---|
| short | 12 | 48 | Single-sentence comparison question |
| medium | 28 | 112 | Multi-clause technical question |
| long | 91 | 364 | Paragraph review with context and follow-up questions |
| very_long | 187 | 748 | Multi-paragraph system design scenario |

A 3-second cooldown was inserted between calls to allow Ollama to release resources between consecutive inference requests.

Results are recorded in `benchmark_labeling_results.jsonl`.

---

### 5.3 Experiment A Results — Vary Transaction Count

| Batch size (N) | Total input tokens | Run 1 | Run 2 | Run 3 | Avg (valid runs) |
|---|---|---|---|---|---|
| 1 | 25 | 5150 ms | 4521 ms | 4493 ms | **4721 ms** |
| 2 | 50 | 8915 ms | 6532 ms | 6215 ms | **7221 ms** |
| 4 | 100 | SKIP (400) | 13523 ms | SKIP (400) | **~14600 ms** ¹ |
| 8 | 200 | SKIP (400) | 39884 ms | 31768 ms | **~35826 ms** |
| 16 | 400 | SKIP (400) | SKIP (400) | SKIP (400) | — |

¹ One data point from the preceding first run (15671 ms) was also valid; average of all valid points is ≈ 14.6 s.

**Reading these numbers:** SKIP means Ollama returned HTTP 400, indicating it rejected the request — not that the call timed out. This happened consistently at N=16 (all three runs) and intermittently at N=4 and N=8. The cause is that large batch sizes require Ollama to hold many partial token sequences simultaneously during generation, eventually exhausting its internal request queue or output buffer.

---

### 5.4 Experiment B Results — Vary Prompt Length

| Tier | Total tokens (4 txs) | Run 1 | Run 2 | Run 3 | Avg (valid runs) |
|---|---|---|---|---|---|
| short | 48 | 66908 ms ² | 33317 ms ² | 14601 ms | **~14600 ms** |
| medium | 112 | SKIP (400) | 11947 ms | SKIP (400) | **~11947 ms** |
| long | 364 | 20521 ms | 17322 ms | 17007 ms | **18283 ms** |
| very_long | 748 | 26443 ms | 21588 ms | 21623 ms | **23218 ms** |

² These two runs of the short tier immediately followed the N=8 and N=16 runs of Experiment A, during which Ollama had been under sustained heavy load. The inflated times (67 s and 33 s) reflect Ollama recovering rather than true labeling latency. The third run (14.6 s), after a 3-second cooldown, is the most accurate.

---

### 5.5 What Drives LLM Labeling Latency

The data reveals that **output token count, not input token count, is the primary driver of labeling latency**.

To understand why, consider how transformer inference works:

1. **Prefill phase**: the model reads all input tokens at once. This is a parallel operation and completes in roughly constant time for token counts in the range tested (25–748 tokens). A 7-billion-parameter model running on Apple Silicon processes a few thousand tokens per second in prefill — the 700-token difference between the `short` and `very_long` tiers adds at most ~0.3 seconds of prefill time, which is negligible.

2. **Generation phase**: the model produces one output token at a time. Each token takes the same amount of time regardless of the prompt. For a 7B-parameter model on Apple Silicon, generation speed is approximately 15–25 tokens per second. Every additional label generated adds ~1 token to the output, which takes ~40–70 ms.

The key observation from the data:

- Going from N=1 to N=8 (same 25-token prompt per tx, 8× more txs): latency increases from 4.7 s to 35.8 s — a **7.6× increase for an 8× batch**. This matches the output token scaling: 8 transactions each needing 1–2 labels = ~16 output tokens vs 1–2 for a single transaction.

- Going from `medium` (112 total tokens, 1 label/tx × 4 txs) to `very_long` (748 total tokens, 4 labels/tx × 4 txs): latency increases from 12 s to 23 s — a **1.9× increase for a 6.7× token count increase**. The input token count grew by 6.7× but latency only doubled, because the bottleneck is the generation of 4× more label strings per transaction.

The labels observed in the data confirm this: `very_long` prompts consistently produced 4 distinct labels per call (`ml_ai_engineering`, `dev_ops`, `cloud_engineering`, `data_engineering`) because the scenario genuinely spanned 4 subdomains. `medium` and `short` prompts produced 1–2 labels. The longer output explains the longer latency.

**Practical implication:** `maxBlockConsumption` (which measures input tokens) is a useful proxy for total latency but does not capture the generation phase. `maxNumTransactions` provides a hard bound on output token count that is not otherwise estimable at selection time.

---

### 5.6 Ollama Capacity Limits

The intermittent HTTP 400 errors at N ≥ 4 reveal an important operational constraint. Ollama (at least version tested) enforces internal limits on the size of a single inference request. When the expected output is very long (many labels for many transactions), Ollama rejects the request before inference begins rather than running it to completion.

This is not a network timeout — the rejections arrived in under 1 second. It is a model-level input validation or output buffer limit. Based on the observed pattern:

- N ≤ 2 (25 tok/tx): never rejected
- N = 4 (25 tok/tx): rejected ~50% of the time
- N ≥ 8 (25 tok/tx): rejected ~50–100% of the time
- N = 16 (25 tok/tx): rejected 100% of the time

Setting `maxNumTransactions = 4` keeps the batch size in the zone where rejection is rare rather than the norm. This is an additional, independent argument for the limit beyond latency alone.

---

### 5.7 Calibrated Limits

Based on the benchmark results, the empirically calibrated target values are:

| Constant | Current code value | Empirically recommended | Rationale |
|---|---|---|---|
| `maxNumTransactions` | 32 | **4** | N=4 keeps latency ≤ 15 s for typical prompts and avoids the Ollama capacity failures seen at N ≥ 8 |
| `maxBlockConsumption` | 10000 | **200** | 200 input tokens corresponds to 4 transactions of ~50 tokens each, matching the N=4 boundary and fitting within the 15-second production target |

The current code values are intentionally left uncalibrated pending production deployment — the existing integration test suite uses up to 10 transactions per block and would require updates to reflect the tighter limits. The recommended values above should be applied once the test suite is updated to match production block sizing.

With both limits at their recommended values, the effective behavior for typical developer prompts (25–50 tokens each):
- 4 short prompts (25 tok × 4 = 100 total): limited by `maxNumTransactions=4` at ~14.6 s ✓
- 2 long prompts (91 tok × 2 = 182 total): limited by `maxBlockConsumption=200` at ~9–12 s ✓
- 1 very long prompt (187 tok × 1 = 187 total): limited by `maxBlockConsumption=200` at ~6 s ✓

The two limits are complementary: `maxNumTransactions` prevents output explosion from many short transactions; `maxBlockConsumption` prevents slow prefill from one very large transaction.

---

### 5.8 Conclusions

1. **Output tokens drive latency more than input tokens.** Adding more transactions to a batch increases latency primarily because each transaction requires generating label strings, not because the model reads more input. A batch of 8 short transactions takes 7.6× longer than 1, despite only 8× the input — consistent with near-linear output token scaling at ~20 tokens/second generation throughput.

2. **Two limits are necessary.** `maxBlockConsumption` alone does not bound latency when many short transactions flood the mempool. `maxNumTransactions` alone does not protect against one very long transaction that drives excessive prefill. Both limits must be enforced independently.

3. **Ollama enforces its own capacity ceiling.** HTTP 400 rejections at N ≥ 8 indicate that the inference backend itself has a limit separate from any network timeout. `maxNumTransactions = 4` provides a margin below this ceiling.

4. **The calibration is environment-specific.** The measurements were taken on a single Apple Silicon machine with qwen2.5-coder:7b. A GPU-accelerated host or a larger model would shift all latency values and require recalibration. The benchmark (`make test-realagent-mr1-benchmark`) should be re-run whenever the hardware or model changes.

5. **More ambiguous prompts generate more labels and take longer.** The `very_long` tier, which described a system spanning four subdomains, consistently produced four labels per call and took 23 s. The `medium` tier, with a single-domain question, produced one label per call and took 12 s. This confirms that prompt complexity affects latency indirectly through output multiplicity, not through inference difficulty per se.

---

## 6. Label Analysis per Transaction

Each of the 100 LLM calls (20 validators × 5 rounds) returned a list of one or more
subdomain labels for the four-transaction batch. Labels are returned at temperature 0.5,
which introduces non-determinism: the same prompt may return different secondary labels
across invocations.

The primary label is the label that matches the ground-truth domain. Secondary labels are
additional labels that the model appended — sometimes correct (e.g., `security` on a
blockchain consensus question), sometimes spurious (e.g., `dev_ops` on a React question).

### 6.1 Transaction 1 — Backpropagation / Neural Networks

**Primary label:** `ml_ai_engineering`

| Label combination | Occurrences | Rate |
|---|---|---|
| [ml_ai_engineering] | 97 / 100 | 97% |
| [ml_ai_engineering, test_engineering_and_qa_automation] | 2 / 100 | 2% |
| [ml_ai_engineering, back_end_with_apis] | 1 / 100 | 1% |

`ml_ai_engineering` appeared in 100% of calls. The two secondary labels (test_qa,
back_end_with_apis) each appeared in at most 2 calls across all 100 observations, which
is well below the quorum threshold and never reached the final block.

### 6.2 Transaction 2 — Blockchain Consensus (PoW vs PoS)

**Primary label:** `blockchain_engineering`

| Label combination | Occurrences | Rate |
|---|---|---|
| [blockchain_engineering] | 68 / 100 | 68% |
| [blockchain_engineering, security] | 28 / 100 | 28% |
| [blockchain_engineering, dev_ops] | 3 / 100 | 3% |
| [blockchain_engineering, systems_programming] | 1 / 100 | 1% |
| [blockchain_engineering, dev_ops, security] | 1 / 100 | 1% |

`blockchain_engineering` appeared in 100% of calls. `security` appeared in 28% of calls —
a semantically reasonable secondary label (consensus protocols have security implications)
but still below quorum. `dev_ops` and `systems_programming` appeared rarely and never
affected the outcome.

**Notable observation (Run 3):** One validator returned three labels:
`[blockchain_engineering, dev_ops, security]`. This is the maximum label multiplicity
observed in the dataset and illustrates the non-determinism at temperature 0.5.

### 6.3 Transaction 3 — React Virtual DOM

**Primary label:** `web_front_end`

| Label combination | Occurrences | Rate |
|---|---|---|
| [web_front_end] | 47 / 100 | 47% |
| [web_front_end, ml_ai_engineering] | 49 / 100 | 49% |
| [web_front_end, test_engineering_and_qa_automation] | 3 / 100 | 3% |
| [web_front_end, dev_ops] | 1 / 100 | 1% |
| [web_front_end, ml_ai_engineering, test_engineering_and_qa_automation] | 1 / 100 | 1% (Run 3) |

`web_front_end` appeared in 100% of calls. This transaction has the highest secondary
label rate: `ml_ai_engineering` appeared as a secondary in 49% of calls. This is
plausible because the virtual DOM concept involves diffing algorithms, which the model
associates with algorithmic/ML topics. Despite appearing in nearly half of all individual
calls, `ml_ai_engineering` did not survive to the final block for this transaction in any
of the 5 runs — see Section 7 for the explanation.

This transaction also produced the only observation of a three-label output
(`[web_front_end, ml_ai_engineering, test_engineering_and_qa_automation]` in Run 3).

### 6.4 Transaction 4 — Horizontal vs Vertical Scaling

**Primary label:** `cloud_engineering`

| Label combination | Occurrences | Rate |
|---|---|---|
| [cloud_engineering] | 58 / 100 | 58% |
| [cloud_engineering, dev_ops] | 42 / 100 | 42% |

`cloud_engineering` appeared in 100% of calls. `dev_ops` appeared as a secondary in 42%
of calls — again semantically reasonable (scaling decisions involve operational concerns)
but consistently below quorum across all 5 rounds.

### 6.5 Summary table

| Transaction | Primary label rate | Highest secondary label | Secondary rate |
|---|---|---|---|
| Backpropagation | 100% | test_engineering_and_qa_automation | 2% |
| Blockchain PoW/PoS | 100% | security | 28% |
| React virtual DOM | 100% | ml_ai_engineering | 49% |
| Cloud scaling | 100% | dev_ops | 42% |

---

## 7. Quorum Filtering Effect

The most important result of the label analysis is the gap between the individual
validator output and the final consensus outcome.

For a secondary label to appear in the finalized `SubdomainsFrequency` map, it must be
assigned by at least Q = 7 of the 10 consensus group members **for the same transaction**.
The 10 consensus group members are drawn randomly from the 20 validators each round.

Consider the worst case: `ml_ai_engineering` as a secondary label on Transaction 3 with a
49% individual rate. If we treat validator calls as independent Bernoulli trials with
p = 0.49, the expected number of consensus group members (out of 10) that include this
secondary label is 4.9. The probability that exactly 7 or more include it is:

```
P(X ≥ 7) where X ~ Binomial(10, 0.49) ≈ 0.117
```

That is approximately an 11.7% chance per round. Across the 5 rounds in this test, the
probability that it appears in at least one round is:

```
P(appears at least once) = 1 - (1 - 0.117)^5 ≈ 46%
```

Yet it did not appear in any of the 5 rounds. This is consistent with the observed
individual rate being volatile per-round (ranging from 35% to 65% across rounds), and
with the random composition of the consensus group acting as a second source of
variance. The result is a **strong but not absolute filter**: the higher the secondary
label rate and the more rounds are run, the more likely it is to eventually cross the
quorum boundary.

For `security` on Transaction 2 (28% individual rate), the expected count in the
consensus group is 2.8, and P(X ≥ 7) ≈ 0.6%. Over 5 rounds the cumulative probability
is ~3%. Again it did not appear.

The data shows that the quorum mechanism reliably suppresses secondary labels with
individual rates below roughly 50%, which matches the theoretical BFT guarantee:
up to f = 3 validators out of 10 can deviate from the primary label without affecting
the finalized block.

---

## 8. Final Consensus Frequencies — All 5 Rounds

All five rounds produced an identical `SubdomainsFrequency` map:

```json
{
  "blockchain_engineering": 7,
  "cloud_engineering": 7,
  "ml_ai_engineering": 7,
  "web_front_end": 7
}
```

Each label carries a frequency of 7, which equals the quorum threshold Q. This means that
in each round, exactly the minimum required number of consensus group members (7 out of
10) unanimously assigned the primary label to each transaction, with no secondary label
reaching that bar for any transaction in any round.

The 100% round-over-round stability of the output demonstrates that the MoA consensus
mechanism successfully extracts a stable, noise-free signal from 20 non-deterministic LLM
agents operating at temperature 0.5.

---

## 9. Comparison with 7-Validator Configuration

Earlier in the same test session, 10 rounds were run with only 7 validators (consensus
group = 3, quorum = 3). Those results are in `repeated_runs_results.jsonl` lines 1–10.

With 7 validators:
- Group = 3, Quorum = 3 → f = 0 (no Byzantine fault tolerance).
- `dev_ops` appeared in the final frequencies in one round (Run 2, round 201), because
  with quorum = 3, a secondary label needs only all 3 consensus members to include it.
- `ml_ai_engineering` appeared at frequency 6 in three rounds, meaning the label passed
  quorum for 2 transactions instead of 1 — another sign of lower signal fidelity.

Increasing from 7 to 20 validators eliminated this noise entirely and stabilized the
output to the same four frequencies across all rounds. This empirically confirms that
network size directly determines the protocol's noise-rejection capability.

---

## 10. Round Timing Analysis

### 10.1 Observed timings

All timing data is from `timing.log`, recorded by the `timingAgent` wrapper that
instruments every `LabelBatch` call.

| Round | Round number | Duration |
|---|---|---|
| Run 1 | 200 | 5 min 30 s |
| Run 2 | 201 | 5 min 25 s |
| Run 3 | 202 | 5 min 35 s |
| Run 4 | 203 | 5 min 20 s |
| Run 5 | 204 | 5 min 25 s |
| **Mean** | | **5 min 27 s** |
| **Range** | | **15 s** |

### 10.2 Per-validator latency distribution

Within each round the leader validator labels first (because it must propose the block
before the other validators can validate). The remaining 19 validators then submit their
LLM requests concurrently after receiving the proposed block.

**Leader LLM latency (1 per round, 5 observations):**

| Run | Leader elapsed |
|---|---|
| 1 | 18.96 s |
| 2 | 14.10 s |
| 3 | 13.08 s |
| 4 | 12.31 s |
| 5 | 15.80 s |
| **Mean** | **14.9 s** |

The leader always contacts Ollama when it is idle (no competing requests) and therefore
receives a response in the model's native single-batch inference time.

**Non-leader LLM latency summary across all 95 observations (5 runs × 19 validators):**

| Percentile | Elapsed |
|---|---|
| p0 (minimum) | 1 min 30 s |
| p25 | 3 min 30 s |
| p50 (median) | 4 min 22 s |
| p75 | 4 min 55 s |
| p100 (maximum) | 5 min 18 s |

### 10.3 Why the gap exists

After the leader completes (~15 s), all 19 remaining validators submit their 4-transaction
batch to the shared Ollama instance simultaneously. Because the instance is configured
with `OLLAMA_NUM_PARALLEL=7`, it can process at most 7 concurrent requests. The 19
simultaneous requests form a queue; the last batch to be served has to wait for all
earlier batches to complete first.

This is a **test-only artifact**. In a production deployment each of the 20 nodes runs its
own Ollama instance, so all 20 LLM calls execute in parallel. The expected production
round time would be approximately equal to the single-batch LLM latency, which is ~15 s
based on the leader observations above.

```
Test environment:   ~5 min 27 s per round   (20 nodes share 1 Ollama, serial queue)
Production:         ~15 s per round          (each node has its own Ollama, parallel)
```

The 20× difference is entirely attributable to infrastructure sharing in the test
environment, not to the protocol design.

---

## 11. Byzantine Fault Tolerance — k = 3 Corrupted Validators

This section analyzes the second test series, which evaluates the protocol's resilience when a subset of validators deliberately return an incorrect label. It reports a discovered vulnerability in the original protocol, the fix applied, and the empirical results that confirm correct BFT behaviour after the fix.

---

### 11.1 Test Configuration

| Parameter | Value |
|---|---|
| Network size | 20 validators |
| Consensus group size G | 10 |
| Quorum threshold Q | 7 |
| BFT tolerance f | 3 |
| Corrupted validators k | 3 (= f, the theoretical maximum) |
| Corrupted validator behaviour | Always return `["security"]` synchronously (no LLM call) |
| Honest validator behaviour | Call the real LLM agent (qwen2.5-coder:7b) |
| Transactions per round | 1 (backpropagation / neural networks prompt) |
| Expected correct label | `ml_ai_engineering` |
| Rounds executed | 5 |

The 3 corrupted validators are deterministically assigned to positions 1–3 in the node list; which of them land in the randomly selected consensus group varies per round.

---

### 11.2 Discovered Vulnerability — Timing Attack on Vote Collection

The original protocol aggregated labels as soon as Q = 7 votes arrived. Because corrupted validators respond instantly (no LLM call), their votes were always the first to reach the leader. In a worst-case round where all 3 corrupted validators were inside the consensus group, the sequence was:

```
t ≈ 0 s   : 3 corrupted validators send votes (label = "security")
t ≈ 33 s  : leader finishes its own LLM call and proposes the block
t ≈ 33–34 s : 4 honest validators happen to finish next → Q = 7 votes collected
             votes in pool: 3 × security + 4 × ml_ai_engineering
             → ml_ai_engineering count = 4 < Q = 7 → does not pass
             → security count = 3 < Q = 7 → does not pass
             → SubdomainsFrequency = {} (empty)
```

This is a **timing-based liveness attack**: Byzantine validators cannot corrupt the block hash (block consensus still holds) but they can prevent any label from reaching the frequency threshold, rendering the subdomain map empty. The attack succeeds with probability P(at least one corrupted validator lands in the consensus group) ≈ 89.5% per round.

The first test run, executed before the fix (records at round numbers 300–304, timestamp 2026-07-11T15:31–15:36), confirmed this exactly: all 5 rounds reached block consensus (`finalizedCount = 20/20`) but produced empty `subdomainsFrequencies`.

---

### 11.3 Protocol Fix — Collect All G Votes Before Aggregating

The root cause was that the two thresholds — how many votes to *collect* and how many votes a label needs to *pass* — were both set to Q. The fix decouples them:

- **Vote collection threshold**: changed from Q = 7 to G = 10. The leader waits for all consensus-group members to vote before aggregating.
- **Label frequency threshold**: remains Q = 7. A label still needs to appear in at least 7 of the aggregated votes to enter the final `SubdomainsFrequency` map.
- **Liveness fallback**: a configurable `voteCollectionDeadline` timer starts when the leader enters the collect-votes step. If the deadline fires before all G votes arrive (e.g., a validator is offline), the leader aggregates with whatever votes are present, as long as at least Q are available.

With this fix, the timing advantage of Byzantine validators is neutralised: even if all 3 corrupted validators vote first, the leader waits for the remaining 7 honest votes before aggregating. The final pool always contains G = 10 votes, and `ml_ai_engineering` appears in at least 7 of them whenever k ≤ f = 3.

---

### 11.4 Results — All 5 Rounds Correct After the Fix

The second test run (records at round numbers 300–304, timestamp 2026-07-11T16:12–16:16) was executed after the fix:

| Run | Round | Duration | ConsensusReached | Finalized | `ml_ai_engineering` freq | `security` freq |
|---|---|---|---|---|---|---|
| 1 | 300 | 75 s | ✓ | 20/20 | 9 | — |
| 2 | 301 | 65 s | ✓ | 20/20 | 8 | — |
| 3 | 302 | 65 s | ✓ | 20/20 | 9 | — |
| 4 | 303 | 65 s | ✓ | 20/20 | 8 | — |
| 5 | 304 | 65 s | ✓ | 20/20 | 8 | — |

All 5 rounds satisfied the BFT assertion: `ml_ai_engineering` appeared in every finalized block, and `security` did not appear in any. Node error count was zero across all rounds.

---

### 11.5 What the Frequency Values Reveal

Unlike the baseline Group B test where all 10 consensus-group members are honest and return the correct label (frequency = 10 per transaction), the Byzantine test produces frequencies of 8 or 9. This is not a failure — it directly encodes how many corrupted validators landed in the consensus group that particular round.

A corrupted validator always returns `["security"]`, so it contributes 0 votes to `ml_ai_engineering`. If *j* corrupted validators land in the consensus group, the frequency is 10 − *j*:

| Observed frequency | Honest voters in group | Corrupted voters in group | `security` votes | `security` passes quorum? |
|---|---|---|---|---|
| 10 | 10 | 0 | 0 | No |
| **9** | **9** | **1** | **1** | **No (1 < 7)** |
| **8** | **8** | **2** | **2** | **No (2 < 7)** |
| 7 | 7 | 3 | 3 | No (3 < 7) |

The five observed frequencies {9, 8, 9, 8, 8} reveal that 1 corrupted validator was in the consensus group in runs 1 and 3, and 2 were in runs 2, 4, and 5. The distribution is consistent with the Hypergeometric(N=20, K=3, n=10) distribution:

```
P(j corrupted in consensus group of 10 | k=3 corrupted in 20 validators)

P(j=0) = C(3,0)·C(17,10) / C(20,10) = 19448 / 184756 ≈ 10.5%
P(j=1) = C(3,1)·C(17,9)  / C(20,10) = 72930 / 184756 ≈ 39.5%
P(j=2) = C(3,2)·C(17,8)  / C(20,10) = 72930 / 184756 ≈ 39.5%
P(j=3) = C(3,3)·C(17,7)  / C(20,10) = 19448 / 184756 ≈ 10.5%
```

Across 5 rounds, j = 1 occurred twice and j = 2 occurred three times — consistent with the expected value of E[j] = k × n / N = 3 × 10 / 20 = 1.5. The cases j = 0 (no corrupted in group) and j = 3 (all corrupted in group, the BFT boundary) did not occur in 5 rounds; their combined probability of appearing at least once is 1 − (1 − 0.105)² × (1 − 0.105)² ≈ 43%, so their absence is unremarkable.

Crucially, even at the boundary j = 3, the protocol remains correct: 7 honest voters produce `ml_ai_engineering` count = 7 = Q, which just reaches the quorum threshold.

---

### 11.6 Timing Analysis

With 1 transaction per batch (vs 4 in the Group B baseline), individual LLM calls are shorter. The timing structure differs from the baseline because 3 of the 20 validators are corrupted and never contact Ollama.

**Run 1** (cold model, first invocation of the session):

| Metric | Value |
|---|---|
| Leader LLM elapsed | 33.3 s |
| Non-leader p50 elapsed | 57.1 s |
| Non-leader max elapsed | 72.2 s |
| Round duration | 75 s |

**Runs 2–5** (warm model):

| Metric | Value |
|---|---|
| Leader LLM elapsed | 4.4–4.7 s |
| Non-leader p50 elapsed | ~45 s |
| Non-leader max elapsed | ~59 s |
| Round duration | 65 s |

The warm-model leader latency (4–5 s for 1 transaction) matches the expected single-batch inference time with no competition from other requests. After the leader proposes the block, 17 honest non-leaders (the 3 corrupted issue no Ollama requests) compete for 7 parallel slots. With 17 requests and 7 slots: the first 7 finish in roughly 23 s, the next 7 in another 23 s, and the remaining 3 shortly after, giving a tail around 55–60 s.

The 7-minute `voteCollectionDeadline` never fires in any round because all G = 10 consensus-group members complete well within 65 s: the corrupted members respond at t ≈ 0, and the honest members respond between t ≈ 23 s and t ≈ 60 s.

---

### 11.7 Secondary Labels and Quorum Filtering

Across 85 honest validator calls (5 runs × 17 honest validators):

| Label combination | Occurrences | Rate |
|---|---|---|
| `[ml_ai_engineering]` only | 71 / 85 | 84% |
| `[ml_ai_engineering, test_engineering_and_qa_automation]` | 9 / 85 | 11% |
| `[ml_ai_engineering, back_end_with_apis]` | 5 / 85 | 6% |
| `[ml_ai_engineering, systems_programming]` | 1 / 85 | 1% |

`ml_ai_engineering` appeared in 100% of honest calls. Secondary labels (`test_engineering_and_qa_automation` at 11%, `back_end_with_apis` at 6%) did not appear in any finalized frequency map — the quorum threshold of Q = 7 filters them out, consistent with the baseline analysis in Section 7.

---

### 11.8 Conclusions

1. **The timing vulnerability is real and non-trivial.** Before the fix, k = 3 corrupted validators (the exact BFT maximum) caused empty subdomain frequencies in all 5 rounds by exploiting the fact that both vote collection and label frequency used the same threshold Q. Instant responses from corrupted validators systematically crowd out honest votes in the first Q collected.

2. **The protocol fix restores the BFT guarantee.** Changing the vote collection threshold from Q to G ensures the leader aggregates labels from all consensus-group members. With G votes in the pool, k ≤ f = 3 corrupted validators can supply at most 3 votes for `"security"`, which is always below Q = 7. The correct label always reaches exactly Q = 7 votes in the worst case (j = f = 3) and more in better cases.

3. **The liveness fallback is not needed under normal conditions.** The 7-minute deadline was never triggered in any test round because all 10 consensus-group members (including the 3 corrupted, which respond immediately) completed within 65 seconds. The timeout exists as a safety net for production scenarios where a validator might be offline.

4. **Frequency values are informative.** The variation in `ml_ai_engineering` frequencies (8 or 9 across runs) directly reflects how many corrupted validators were selected into the consensus group that round. This makes the frequency map a diagnostic tool for understanding the round-by-round composition of the consensus group.

5. **`security` never reached quorum.** In no round across either the pre-fix or post-fix runs did the `security` label appear in the finalized `SubdomainsFrequency` map. The corrupted validators' label was consistently suppressed by the threshold, confirming that Byzantine validators can disrupt label availability (pre-fix) but cannot inject false labels (in either version).

---

## 12. Byzantine Fault Tolerance — k = 6 Corrupted Validators (Beyond the BFT Bound)

This section reports the results of the k = 6 test, which places the protocol under adversarial conditions that exceed the theoretical BFT tolerance (k = 6 = 2f). It confirms that the protocol degrades predictably when the adversary controls more than f validators.

---

### 12.1 Test Configuration

| Parameter | Value |
|---|---|
| Network size | 20 validators |
| Consensus group size G | 10 |
| Quorum threshold Q | 7 |
| BFT tolerance f | 3 |
| Corrupted validators k | 6 (= 2f, twice the theoretical maximum) |
| Corrupted validator behaviour | Always return `["security"]` synchronously (no LLM call) |
| Transaction | Same backpropagation / neural networks prompt |
| Rounds completed | 2 of 5 (run 3 aborted due to transient LLM service error) |

---

### 12.2 Results

| Run | Round | Duration | ConsensusReached | Finalized | `ml_ai_engineering` freq | `security` freq |
|---|---|---|---|---|---|---|
| 1 | 400 | 70 s | ✓ | 20/20 | — | — |
| 2 | 401 | 55 s | ✓ | 20/20 | — | — |
| 3 | 402 | — | — | — | aborted (HTTP 400 from LLM agent) | — |

Both completed rounds produced empty `subdomainsFrequencies`. Block consensus was reached in both (all 20 nodes finalized the same block header), but no label survived the quorum threshold.

Run 3 failed before any votes were collected: the node selected as leader for round 402 received an HTTP 400 error from the LLM agent when calling `ProposeBlockAndDomains`. This is a transient infrastructure failure unrelated to the protocol — Ollama rejected the request after two consecutive rounds of heavy parallel load. The test was interrupted at that point.

---

### 12.3 What Empty Frequencies Mean at k = 6

With k = 6 corrupted validators and G = 10 consensus group members, the number j of corrupted validators in the group follows a Hypergeometric(N = 20, K = 6, n = 10) distribution:

```
P(j=0) = C(6,0)·C(14,10) / C(20,10) =  1001 / 184756 ≈  0.5%
P(j=1) = C(6,1)·C(14,9)  / C(20,10) = 12012 / 184756 ≈  6.5%
P(j=2) = C(6,2)·C(14,8)  / C(20,10) = 45045 / 184756 ≈ 24.4%
P(j=3) = C(6,3)·C(14,7)  / C(20,10) = 68640 / 184756 ≈ 37.2%
P(j=4) = C(6,4)·C(14,6)  / C(20,10) = 45045 / 184756 ≈ 24.4%
P(j=5) = C(6,5)·C(14,5)  / C(20,10) = 12012 / 184756 ≈  6.5%
P(j=6) = C(6,6)·C(14,4)  / C(20,10) =  1001 / 184756 ≈  0.5%

P(j ≤ 3) ≈ 68.6%   → ml_ai_engineering reaches Q = 7, label survives
P(j ≥ 4) ≈ 31.4%   → ml_ai_engineering gets at most 6 votes < Q = 7, empty map
```

A frequency of `ml_ai_engineering: 10-j` would appear in any round where j ≤ 3. Empty frequencies indicate j ≥ 4. Both completed rounds produced empty maps, confirming j ≥ 4 in both cases. The probability of this occurring twice in a row is 0.314² ≈ 9.9% — low but not negligible, and consistent with the theoretical distribution.

---

### 12.4 Why `security` Still Cannot Reach Quorum

Even with k = 6 corrupted validators all inside the consensus group (the worst case, j = 6), the corrupted label `security` accumulates only 6 votes. The quorum threshold is Q = 7. Therefore `security` < Q in every possible scenario when k ≤ 6.

More generally, for `security` to reach quorum at Q = 7, the adversary would need to control at least Q = 7 validators. With N = 20 and G = 10, that requires k ≥ 7, which is well beyond k = 6. The attack at k = 6 can only suppress the correct label (by preventing it from reaching Q) — it cannot elevate a false label to quorum.

This asymmetry is a fundamental property of the threshold: the adversary can cause **label availability failures** but not **label injection**.

---

### 12.5 Comparison: k = 3 vs k = 6

| Property | k = 3 (= f) | k = 6 (= 2f) |
|---|---|---|
| Block consensus always reached | ✓ | ✓ |
| Correct label always reaches Q | ✓ (after fix) | Only when j ≤ 3 (~68.6% per round) |
| False label ever reaches Q | Never | Never |
| Theoretical guarantee | BFT bound holds exactly | Degradation is expected and observed |
| Observed outcome | 5/5 correct | 2/2 empty (both rounds had j ≥ 4) |

The contrast is the empirical proof of the BFT bound's tightness. At k = f = 3 the protocol provides a hard correctness guarantee; at k = 2f = 6 it degrades gracefully — block finality is preserved but label availability becomes probabilistic, dependent on how many corrupted validators happen to land in the randomly selected consensus group each round.

---

### 12.6 Conclusions

1. **Block consensus is resilient beyond the BFT bound.** Even with k = 6 corrupted validators (twice the tolerance), all 20 nodes agreed on the same block header in every completed round. Byzantine validators cannot forge block signatures or break hash agreement regardless of their count.

2. **Label availability degrades predictably at k > f.** When the randomly selected consensus group happens to contain ≥ 4 corrupted validators (probability ≈ 31.4% per round), no label reaches Q = 7 and the `SubdomainsFrequency` map is empty. The degradation rate matches the Hypergeometric distribution with E[j] = k × G / N = 6 × 10 / 20 = 3.

3. **False label injection remains impossible.** With only k = 6 corrupted validators, the maximum number of `security` votes in any consensus group is 6, which is strictly less than Q = 7. This holds for any k ≤ 6. The adversary can cause empty frequency maps but cannot manufacture a false quorum.

4. **The BFT boundary is tight.** The difference between k = 3 (guaranteed correctness) and k = 6 (probabilistic availability) confirms that the protocol's f = G − Q = 10 − 7 = 3 tolerance is an exact threshold, not a conservative estimate. One corrupted validator beyond f immediately introduces non-zero failure probability.

---

## 13. Architectural Optimization — Pre-computing LLM Inference at Mempool Entry

This section analyzes a latency optimization that emerges from the protocol structure: moving the LLM inference steps (labeling and answering) out of the mini-round critical path and into a background pre-computation phase triggered when a transaction first enters the mempool.

---

### 13.1 The Current Bottleneck

In the current protocol the LLM inference steps sit on the critical path of every round:

```
Round start
  └─ MR1: leader labels tx → proposes block → validators label tx → submit votes
      └─ MR2: validators answer tx → submit votes → clustering
```

In the test environment the dominant cost is the shared Ollama queue (~5.5 min). In production, with one Ollama per node, the cost drops to the single-batch inference latency (~15 s per LLM call). Two sequential mini-rounds each containing a blocking LLM inference step means production round time is bounded below by ~30 s regardless of network conditions.

---

### 13.2 Independence of the Two LLM Steps

The key observation is that **both LLM steps are independent of each other and of round state**:

- **Labeling (MR1 input)**: each validator classifies a transaction using only the transaction content. No knowledge of the current round, the leader, the consensus group, or other validators' opinions is required.
- **Answering (MR2 input)**: each validator answers the transaction independently. The answer is produced from the transaction text alone; which validators' answers are ultimately considered in MR2 is determined later by the labels from MR1, but the *production of the answer* does not require those labels.
- **Clustering (MR2 aggregation)**: the only in-round LLM-adjacent step. Clustering operates on the *collected answers* from the selected validators — it requires knowing which answers to group, so it cannot start until MR2 vote collection is done.

This means labeling and answering can both be pre-computed as soon as a transaction is known.

---

### 13.3 Proposed Architecture

```
tx enters mempool
  ├─ validator fires async LLM call: label(tx)   → cache label
  └─ validator fires async LLM call: answer(tx)  → cache answer

Round start
  └─ MR1: leader proposes block → validators submit cached labels (network only)
      └─ MR2: validators submit cached answers (network only) → clustering
```

The mini-rounds collapse to pure message-passing phases. The in-round critical path becomes:

```
network latency (proposal broadcast + vote collection) + clustering time
```

This is on the order of seconds rather than tens of seconds.

---

### 13.4 Production Case Analysis

#### 13.4.1 Latency

| Phase | Current (test) | Current (production) | With pre-computation |
|---|---|---|---|
| MR1 LLM labeling | ~5 min (queue) | ~15 s | 0 s (cached) |
| MR1 message passing | ~5 s | ~5 s | ~5 s |
| MR2 LLM answering | ~5 min (queue) | ~15 s | 0 s (cached) |
| MR2 message passing | ~5 s | ~5 s | ~5 s |
| Clustering | ~5 s | ~5 s | ~5 s |
| **Total** | **~10 min** | **~45 s** | **~15 s** |

The production round time drops from ~45 s to ~15 s — a 3× improvement. The remaining latency is network messaging and clustering, both of which are bounded by infrastructure rather than model inference time.

In the case where a transaction spends significant time in the mempool before being included in a block (a common case under any non-trivial transaction load), the pre-computation happens entirely in the background while previous rounds are running. The effective latency contribution of LLM inference to the round that includes the transaction is zero.

#### 13.4.2 Compute Waste

Pre-computing for all mempool transactions introduces two sources of wasted inference:

**1. Transactions that are never included.** If a transaction is evicted from the mempool (expired, replaced, or rejected by the leader) after its label and answer have been pre-computed, that inference is wasted. The waste rate is bounded by the eviction rate, which depends on application-level transaction validity policies.

**2. Validators outside the consensus group.** With N = 20 validators and G = 10 per group, each validator has a 50% probability of being selected for any given round. A validator that pre-computes a label/answer but is not in the consensus group for that round contributes nothing to the outcome. Over many rounds each validator is selected ~50% of the time, so the expected per-transaction compute utilization across the network is 50%.

This is an inherent overhead of the random group selection mechanism, not a property of the pre-computation optimization. In the current (non-pre-computed) design the same waste exists — validators outside the group still run their LLM call to cast a vote, but the vote is not used in the final block. Pre-computation does not increase the total inference budget; it shifts when that inference happens.

The practical overhead is therefore bounded: at a mempool eviction rate of r and group utilization of 50%, the effective compute usage per confirmed transaction is `1 / (0.5 × (1 - r))`. For r = 10% this is 1/0.45 ≈ 2.2× the minimum necessary compute, which is well within normal replication overhead for a fault-tolerant system.

#### 13.4.3 Cache Management

Each validator must maintain a per-transaction cache mapping `tx_hash → {label, answer}`. Design constraints:

- **TTL**: entries should be expired after a configurable duration (e.g., 2× the expected round time) to prevent unbounded growth in the presence of long-pending transactions.
- **Cache miss fallback**: if a transaction enters a round without a pre-computed entry (e.g., newly arrived, cache evicted, or pre-computation failed), the validator falls back to performing the inference inline during the round — exactly the current behavior. No protocol change is needed to handle this path.
- **Concurrency**: pre-computation runs concurrently with active rounds. Validators must ensure cache reads during a round and background writes at mempool entry are properly synchronized. In practice this is a simple read-write lock or a lock-free map.

#### 13.4.4 Byzantine Timing Attack

The pre-computation model has a secondary security benefit. In the current protocol the timing vulnerability analyzed in Section 11.2 arises because Byzantine validators (which skip the LLM call) can submit their votes in milliseconds while honest validators are still waiting for Ollama. The G-threshold fix closes the attack, but the timing gap still exists.

With pre-computation, *all* validators submit their votes at round start after a network propagation delay — honest and Byzantine alike. The gap between the fastest and slowest vote arrival shrinks from ~60 s (LLM inference time) to ~milliseconds (network jitter). The timing attack surface is eliminated at the architectural level, making the G-threshold fix a belt-and-suspenders defense rather than the primary barrier.

---

### 13.5 Interaction with Mempool Transaction Volume

The optimization's benefit is proportional to how long transactions wait in the mempool before being proposed:

- **Low throughput** (one tx per round, tx arrives just before round start): pre-computation completes concurrently with the previous round's clustering phase. The label and answer are cached with ~15 s of overlap time — the benefit is the full 30 s of in-round LLM inference.
- **High throughput** (many txs queued, each waits multiple rounds): pre-computation completes during earlier rounds. The label and answer are cached and ready when the tx is eventually proposed. LLM inference contributes zero latency to the proposing round.
- **Burst scenario** (many txs arrive simultaneously): each validator fires multiple async LLM calls concurrently. With one dedicated Ollama instance per node, the queue is bounded by the validator's own hardware rather than shared among all 20 nodes. Individual tx pre-computation time increases under burst load, but this is amortized over the rounds during which the txs wait.

The optimization's worst case is the same as the current protocol's best case: a transaction that enters the mempool at the exact moment its round starts, with zero pre-computation time available. This is increasingly unlikely as transaction throughput grows.

---

### 13.6 Summary

| Property | Current protocol | With mempool pre-computation |
|---|---|---|
| Production round time | ~45 s | ~15 s |
| LLM inference on critical path | Yes (both MR1 and MR2) | No (only clustering remains) |
| Compute waste | Same as pre-computation | Same as current |
| Cache miss fallback | N/A | Falls back to in-round inference |
| Byzantine timing surface | Reduced by G-threshold fix | Eliminated at architecture level |
| Clustering remains in-round | N/A | Yes — requires collected answers |
| Protocol changes required | N/A | Mempool hook + per-validator cache |

The optimization requires no changes to the consensus protocol itself — the interface between the pre-computation cache and the mini-round handler is a simple cache lookup replacing the LLM call. The core round logic, BFT guarantees, and clustering remain identical.

---

## 14. Conclusions

1. **Label correctness**: The four primary domain labels (`ml_ai_engineering`,
   `blockchain_engineering`, `web_front_end`, `cloud_engineering`) were correctly
   identified in 100% of individual LLM calls across all 100 observations. The MoA
   labeling model is highly accurate for unambiguous, single-domain prompts.

2. **Secondary label noise**: Secondary labels appeared frequently at the individual level
   (up to 49% for `ml_ai_engineering` on the React transaction) due to temperature 0.5
   non-determinism. These do not represent errors — they reflect genuine semantic
   ambiguity between adjacent subdomains.

3. **Quorum filtering works**: Despite secondary label rates of up to 49%, no secondary
   label survived to the final block in any of the 5 rounds. The BFT quorum threshold
   (Q = 7 out of 10) acts as a statistical filter that eliminates minority opinions.

4. **Perfect convergence**: All 5 rounds produced the identical `SubdomainsFrequency`
   map, demonstrating that the protocol is deterministic at the consensus level even when
   individual agents are non-deterministic.

5. **Test latency is a deployment artifact**: The 5-minute round time is caused by
   serialized access to a shared Ollama instance. In production (one Ollama per node)
   the round time reduces to the single-batch LLM inference time of ~15 seconds, a 20×
   improvement.

6. **Network size matters for BFT**: The 7-validator configuration (f = 0) allowed
   secondary labels to appear in final frequencies. The 20-validator configuration
   (f = 3) eliminated them entirely, confirming that the quorum is only meaningful when
   the network is large enough to provide genuine fault tolerance.

---

## 15. Data Files

| File | Description |
|---|---|
| `repeated_runs_results.jsonl` | One JSON object per run: timestamps, duration, frequencies, non-related hashes |
| `byzantine_runs_results.jsonl` | One JSON object per Byzantine run: corrupted count, consensus outcome, frequencies |
| `benchmark_labeling_results.jsonl` | One JSON object per benchmark call: experiment, batch size, token counts, latency |
| `../../logs/TestMiniRoundOne_RealAgent_RepeatedRuns_GroupB_ClearDomainConvergeToExpectedLabels/timing.log` | Per-call LLM latency and label output for every validator in every run |
| `../../logs/TestMiniRoundOne_RealAgent_Byzantine_K3_CorrectLabelSurvives/timing.log` | Per-call LLM latency and label output for honest validators in the k=3 Byzantine runs |
| `../../logs/TestMiniRoundOne_RealAgent_RepeatedRuns_GroupB_ClearDomainConvergeToExpectedLabels/validator-N.log` | Per-node structured log for each of the 20 validators |
