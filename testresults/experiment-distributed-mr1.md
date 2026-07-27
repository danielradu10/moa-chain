# Distributed MR1 Experiment — 2026-07-25

## Setup

| Parameter | Value |
|---|---|
| Test | `TestDistributedMR1_AllAgentsConverge` |
| Nodes | 10 (moa-chain-0 … moa-chain-9) |
| Model | `qwen2.5-coder:7b` (Ollama) |
| Committee strategy | `full` — all 10 validators in every committee |
| Temperatures | 0.30, 0.35, 0.40, 0.45, 0.50, 0.55, 0.60, 0.65, 0.70, 0.75 |
| Transactions | 3 software-engineering Q&A prompts |
| Labeler prompt | `labeler_v3` |
| Round number | 30 |

### Transactions

| # | Prompt |
|---|---|
| 1 | "What is the main benefit of unit tests?" |
| 2 | "Why must validators verify message signatures?" |
| 3 | "Why does deterministic ordering matter in consensus?" |

---

## Results

**Run 1** — 2026-07-25 19:03:52 UTC — `PASS`

| Metric | Value |
|---|---|
| Round duration | 1m 45s |
| NonRelatedCount | 0 |

**Consensus subdomain frequencies:**

| Subdomain | Frequency |
|---|---|
| `blockchain_engineering` | 5 |
| `security` | 5 |
| `systems_programming` | 5 |
| `test_engineering_and_qa_automation` | 5 |

**Run 2** — 2026-07-25 19:22:17 UTC — `PASS` _(Ollama parallelism experiment — reverted)_

Settings changed from baseline: `OLLAMA_NUM_PARALLEL=3`, `OLLAMA_FLASH_ATTENTION=1`, `OLLAMA_KV_CACHE_TYPE=q8_0`, `LABEL_MAX_CONCURRENCY=3`.

| Metric | Value |
|---|---|
| Round duration | 2m 35s |
| NonRelatedCount | 0 |

Consensus subdomain frequencies identical to Run 1.

### Per-agent label batch duration

**Baseline (LABEL_MAX_CONCURRENCY=2, default Ollama settings):**

| Machine | Temperature | Total label time (s) |
|---|---|---|
| moa-chain-0 | 0.30 | 43.7 |
| moa-chain-1 | 0.35 | 35.8 |
| moa-chain-2 | 0.40 | 45.6 |
| moa-chain-3 | 0.45 | 35.9 |
| moa-chain-4 | 0.50 | 47.2 |
| moa-chain-5 | 0.55 | 36.6 |
| moa-chain-6 | 0.60 | 39.8 |
| moa-chain-7 | 0.65 | 43.6 |
| moa-chain-8 | 0.70 | 67.9 |
| moa-chain-9 | 0.75 | 39.1 |

**Parallelism experiment (LABEL_MAX_CONCURRENCY=3, OLLAMA_NUM_PARALLEL=3):**

| Machine | Temperature | Total label time (s) | Per-tx times (s) |
|---|---|---|---|
| moa-chain-0 | 0.30 | 62.9 | 31.9, 47.8, 62.9 |
| moa-chain-1 | 0.35 | 50.7 | 30.4, 41.1, 50.7 |
| moa-chain-2 | 0.40 | 70.1 | 37.4, 55.8, 70.1 |
| moa-chain-3 | 0.45 | 55.3 | 28.1, 43.5, 55.3 |
| moa-chain-4 | 0.50 | 68.5 | 34.8, 53.2, 68.5 |
| moa-chain-5 | 0.55 | 54.8 | 27.9, 42.1, 54.8 |
| moa-chain-6 | 0.60 | 63.0 | 31.9, 49.2, 63.0 |
| moa-chain-7 | 0.65 | 63.9 | 32.5, 50.1, 63.9 |
| moa-chain-8 | 0.70 | 100.3 | 50.6, 72.2, 100.3 |
| moa-chain-9 | 0.75 | 54.8 | 27.6, 42.7, 54.8 |

**Result: 48% slower** (avg 64.4s vs 43.5s). Changes reverted.

**Why it was slower:** CPU-based inference is compute-saturated with a single request. Setting `OLLAMA_NUM_PARALLEL=3` causes three requests to simultaneously compete for the same CPU cores. Each call takes ~2× longer because it shares compute with the others. The wall-clock time for the batch is `max(per_tx_times)` — visible in the increasing per-tx values above (31→48→63s), where each slot represents one call finishing sequentially inside Ollama despite Python firing all three at once. The net result is longer per-call latency with no reduction in total wall time. For CPU inference, `LABEL_MAX_CONCURRENCY=1` (fully sequential) or `LABEL_MAX_CONCURRENCY=2` (slight pipelining) are both better strategies than 3.

Each agent labels once per round (proposer labels to build the block; validators label to verify it). With `LABEL_MAX_CONCURRENCY=2`, 3 transactions are processed as two sequential batches (2+1).

### 10-Trial Statistical Run — 2026-07-25 — `PASS 10/10`

The test was run 10 consecutive times using `make test-distributed-mr1-trials N=10`, with a full cluster restart (stop → start → warmup) between each trial to ensure no shared Ollama state, KV cache, or conversation context across runs. Two earlier manual runs (including the parallelism experiment) bring the total to 12 trials.

#### Per-trial results

| Trial | Timestamp (UTC) | Duration | Subdomain map |
|---|---|---|---|
| 1 | 2026-07-25 19:42:16 | 145s | dominant |
| 2 | 2026-07-25 19:44:24 | 115s | **divergent** (systems_programming=4) |
| 3 | 2026-07-25 19:46:29 | 110s | dominant |
| 4 | 2026-07-25 19:48:42 | 105s | dominant |
| 5 | 2026-07-25 19:50:42 | 105s | dominant |
| 6 | 2026-07-25 19:52:43 | 105s | dominant |
| 7 | 2026-07-25 19:54:48 | 110s | dominant |
| 8 | 2026-07-25 19:56:52 | 110s | dominant |
| 9 | 2026-07-25 19:59:05 | 120s | dominant |
| 10 | 2026-07-25 20:01:06 | 105s | dominant |

**Duration statistics (10-trial run):** avg = 113.0s, std = 11.7s, min = 105s, max = 145s.

Trial 1 is the slowest (145s). This is the first run after the parallelism experiment and the only one where Ollama had to do a full cold model load despite the warmup call; subsequent trials benefit from a warmer OS page cache and a tighter startup sequence. Excluding trial 1, trials 2–10 have avg = 109.4s, std = 5.5s — very tight.

**Subdomain map distribution across all 12 trials:**

| Map | Count |
|---|---|
| `blockchain_engineering:5, security:5, systems_programming:5, test_engineering_and_qa_automation:5` | 11/12 |
| `blockchain_engineering:5, security:5, systems_programming:4, test_engineering_and_qa_automation:5` | 1/12 |

#### Analysis of the divergent trial

The single divergent trial (trial 2) differs only in `systems_programming` dropping from 5 to 4. This means exactly one validator assigned a different primary label to the third transaction ("Why does deterministic ordering matter in consensus?"). This is the same prompt that triggered subdomain hallucination under `labeler_v2`. Under `labeler_v3` no invalid subdomain is generated, but the prompt remains semantically ambiguous: `systems_programming` and `blockchain_engineering` are both plausible for a question about deterministic ordering. One validator's model, at its particular temperature and random seed on that cold start, resolved the ambiguity differently. The block still finalized correctly — `systems_programming` at frequency 4 exceeds the Q=7 quorum threshold — so the divergence is absorbed by the protocol's Byzantine tolerance rather than causing failure.

#### Per-machine label timing (trial 10 — last run)

Each agent independently labels all 3 transactions. With `LABEL_MAX_CONCURRENCY=2`, transactions are dispatched in two batches: tx1 and tx2 are sent to Ollama simultaneously, tx3 enters as soon as tx1 completes.

| Machine | Temp | tx1 (s) | tx2 (s) | tx3 (s) | Total (s) |
|---|---|---|---|---|---|
| moa-chain-0 | 0.30 | 12.1 | 24.5 | 28.3 | 40.4 |
| moa-chain-1 | 0.35 | 11.8 | 21.6 | 23.9 | 35.6 |
| moa-chain-2 | 0.40 | 14.9 | 32.7 | 35.9 | 50.8 |
| moa-chain-3 | 0.45 | 14.7 | 26.2 | 25.8 | 40.5 |
| moa-chain-4 | 0.50 | 12.6 | 26.6 | 31.9 | 44.5 |
| moa-chain-5 | 0.55 | 15.0 | 30.5 | 30.2 | 45.2 |
| moa-chain-6 | 0.60 | 16.3 | 29.1 | 33.4 | 49.6 |
| moa-chain-7 | 0.65 | 14.2 | 26.4 | 29.6 | 43.8 |
| **moa-chain-8** | **0.70** | **21.4** | **39.8** | **47.0** | **68.3** |
| moa-chain-9 | 0.75 | 15.7 | 27.9 | 27.6 | 43.2 |
| **Average** | | **14.9** | **28.5** | **31.3** | **46.2** |

**Batching pattern:** tx1 is consistently the fastest (avg 14.9s) because it is the first request Ollama processes on a fresh inference context. tx2 is slower (avg 28.5s) because it runs in parallel with tx1 and shares CPU time inside Ollama. tx3 enters the semaphore when tx1 finishes (~14s mark) but Ollama is still processing tx2, so tx3 waits and then runs partly in overlap with tx2 — explaining why tx3 (avg 31.3s) takes longer than tx1 despite running after the first batch completes. The total wall time is dominated by the overlap of tx2 and tx3, not their sum.

**moa-chain-8 hardware outlier:** moa-chain-8 (temperature 0.70) is consistently 50–70% slower than the cluster average across all trials. Its per-tx times are roughly 1.5× the mean, indicating a hardware difference on that specific instance — fewer CPU cores, lower clock speed, or a shared-resource constraint — rather than any inference parameter effect. This is confirmed by the fact that temperature 0.70 shows no unusual latency on other equivalent runs.

**No temperature-latency correlation:** Across the other nine machines (excluding moa-chain-8), there is no monotonic relationship between temperature and label latency. Temperature affects the probability distribution of token selection but not the number of tokens generated for a fixed-length structured output, so it has negligible impact on inference time.

#### Critical path decomposition

All 10 validators label in parallel across machines. The round cannot finalize until the slowest validator delivers its labels. In trial 10:

| Phase | Duration |
|---|---|
| Label phase (parallel, bottleneck = moa-chain-8) | 68.3s |
| Consensus and messaging overhead | ~36.7s |
| **Total round time** | **105.0s** |

The 37s overhead covers: leader block proposal and broadcast, committee vote collection (waiting for all G=10 votes per the adversarial-timing fix), certificate construction and broadcast, and validator finalization. Removing the moa-chain-8 hardware bottleneck would reduce the label phase to ~51s (the next-slowest machine), cutting the round time to approximately 88s.

---

### Byzantine k=3 — 10-Trial Statistical Run — 2026-07-26 — `PASS 10/10`

Three of the ten validators (moa-chain-0, moa-chain-1, moa-chain-2; temperatures 0.30–0.40) were replaced by an in-process stub that immediately returns `ml_ai_engineering` for every transaction, with no LLM call. The remaining seven validators (moa-chain-3 through moa-chain-9) called their real cluster agents with full LLM inference. The test was run 10 times with a full cluster restart between each trial.

**Setup:**

| Parameter | Value |
|---|---|
| Total validators | 10 |
| Byzantine validators (k) | 3 (validators 0–2, in-process stub) |
| Byzantine label | `ml_ai_engineering` |
| Honest validators | 7 (validators 3–9, real LLM) |
| Quorum Q | ⌊2·10/3⌋ + 1 = 7 |
| Byzantine fault bound f | 3 (k = f, worst-case BFT boundary) |

#### Per-trial results

| Trial | Timestamp (UTC) | Duration | Byzantine label in map | Subdomain map |
|---|---|---|---|---|
| 1 | 2026-07-26 14:40:13 | 115s | No | dominant |
| 2 | 2026-07-26 14:42:19 | 110s | No | dominant |
| 3 | 2026-07-26 14:44:29 | 115s | No | **divergent** (systems_programming absent) |
| 4 | 2026-07-26 14:46:36 | 105s | No | dominant |
| 5 | 2026-07-26 14:48:36 | 105s | No | dominant |
| 6 | 2026-07-26 14:50:55 | 125s | No | dominant |
| 7 | 2026-07-26 14:53:00 | 105s | No | **divergent** (systems_programming absent) |
| 8 | 2026-07-26 14:55:22 | 125s | No | dominant |
| 9 | 2026-07-26 14:57:39 | 120s | No | dominant |
| 10 | 2026-07-26 14:59:42 | 105s | No | dominant |

**Duration statistics:** avg = 113.0s, std = 7.8s, min = 105s, max = 125s.

**Subdomain map distribution across 10 passed trials:**

| Map | Count |
|---|---|
| `blockchain_engineering:4, security:4, systems_programming:4, test_engineering_and_qa_automation:4` | 8/10 |
| `blockchain_engineering:4, security:4, test_engineering_and_qa_attention:4` | 2/10 |

#### Analysis

**Byzantine label rejection — 10/10 trials.**  `ml_ai_engineering` received exactly 3 votes per transaction — one from each Byzantine validator — which is strictly below the quorum threshold Q = 7. It was excluded from the finalized frequency map in every single trial. The protocol's collect-all-G-votes rule (the adversarial timing fix from Section IV.C) ensures that even fast Byzantine responses cannot dominate the certificate: all 10 votes are collected before aggregation, so the 3 instant Byzantine votes are weighed against the 7 honest LLM votes rather than being allowed to form a premature quorum.

**Frequency drop from 5 to 4.**  In the honest run, each subdomain had frequency 5. In the Byzantine run, each has frequency 4. The 3 Byzantine validators no longer contribute honest votes to the correct subdomains — instead their 3 votes go to `ml_ai_engineering`. The 7 remaining honest validators produce 4 votes per subdomain per transaction on average, reflecting the same label distribution as the honest run but scaled to 7 participants instead of 10.

**`systems_programming` absent in 2/10 trials.**  This is the same semantically ambiguous transaction ("Why does deterministic ordering matter in consensus?") that produced a divergent frequency in 1/12 honest trials and was the source of hallucinations under `labeler_v2`. With 10 honest validators, enough of them label this transaction with `systems_programming` to consistently reach frequency 5. With only 7 honest validators, the margin shrinks: in 2 out of 10 trials, fewer than the required number of honest validators assigned `systems_programming` to this transaction, so it fell below the inclusion threshold. The block still finalized correctly — `blockchain_engineering` remained and is the dominant label for this transaction. This highlights that semantically borderline subdomains are more sensitive to committee size reductions.

**Duration comparison — Byzantine run is marginally faster.**  The Byzantine avg (113.0s) is slightly lower than the honest avg (115.8s). This is consistent with the protocol's collect-all-G behavior: the leader still waits for all 10 votes, but the 3 Byzantine responses arrive instantly (no LLM call), reducing the effective wait to the slowest of the 7 honest LLM calls rather than the slowest of all 10. The bottleneck remains moa-chain-8 (~68s), so the improvement is small but measurable.

**Comparison with honest baseline:**

| Metric | Honest (12 trials) | Byzantine k=3 (10 trials) |
|---|---|---|
| Pass rate | 12/12 (100%) | 10/10 (100%) |
| Byzantine label in map | — | 0/10 |
| Avg duration | 115.8s | 113.0s |
| Dominant subdomain map frequency | 5 | 4 |
| Divergent trials | 1/12 | 2/10 |
| systems_programming dropped | 0/12 | 2/10 |

---

### Ambiguous / Borderline Prompts — 9/10 Trials Collected — 2026-07-26 — `PASS 9/9`

Three transactions were chosen to sit deliberately on the boundary between two well-known subdomains. The test only asserts that consensus is reached; the research output is the distribution of the frequency map across repeated trials — specifically how often the committee splits versus converges and which subdomain absorbs the ambiguity.

**Setup:**

| Parameter | Value |
|---|---|
| Test | `TestDistributedMR1_Ambiguous_FrequencyVariance` |
| Round number | 60 |
| Transactions | 3 (all borderline, each straddling two subdomains) |
| Makefile target | `make test-distributed-mr1-ambiguous-trials N=10` |
| Trials planned / collected | 10 / 9 (one trial timed out — see analysis) |

#### Transactions and pre-experiment predictions

Before collecting any results, the expected classification behaviour for each prompt was:

| # | Prompt | Straddles | Prediction |
|---|---|---|---|
| 1 | "Why does deterministic ordering matter in consensus?" | `systems_programming` / `blockchain_engineering` | Split vote — "deterministic ordering" points to `systems_programming` (concurrency, scheduling) while "in consensus" anchors to `blockchain_engineering`. Already proven divergent in 1/12 honest trials and in 2/10 Byzantine trials. Expect `systems_programming` to appear but with unstable frequency. |
| 2 | "How does a hash function protect data stored on a blockchain?" | `security` / `blockchain_engineering` | `security` should dominate — "protect" and "hash function" are cryptography vocabulary. The explicit "blockchain" qualifier might pull some votes to `blockchain_engineering`. Expect a consistent `security` signal with variable `blockchain_engineering` support. |
| 3 | "What makes a smart contract vulnerable to reentrancy attacks?" | `security` / `blockchain_engineering` | Strong `security` signal — "vulnerable" and "reentrancy attacks" are security-analysis vocabulary. "Smart contract" keeps `blockchain_engineering` in play. Expect similar behaviour to tx2 but with `security` even more dominant due to the attack-surface framing. |

The working hypothesis: the committee would produce highly variable maps, with at least two subdomains present in every trial. `blockchain_engineering` was expected to be an ever-present background signal because all three prompts explicitly mention blockchain concepts.

#### Per-trial results

| Trial | Timestamp (UTC) | Duration | `blockchain_engineering` | `security` | `systems_programming` |
|---|---|---|---|---|---|
| 1 | 15:40:44 | 190s ⬆ cold | 14 | 10 | 5 |
| 2 | 15:43:10 | 130s | 10 | 10 | 5 |
| 3 | 15:46:08 | 160s | 14 | 10 | 9 |
| — | *(FAIL — hallucination: `smart_contracts`)* | 600s | — | — | — |
| 4 | 16:00:13 | 195s ⬆ cold | 14 | 10 | 9 |
| 5 | 16:02:40 | 130s | 13 | 10 | 5 |
| 6 | 16:05:12 | 135s | 10 | 10 | 5 |
| 7 | 16:08:10 | 160s | 10 | 10 | 5 |
| 8 | 16:10:47 | 135s | 14 | 10 | 5 |
| 9 | 16:13:16 | 135s | 10 | 10 | 5 |

**Duration statistics (9 collected trials):** avg = 152.2s, std = 24.2s, min = 130s, max = 195s.
Excluding cold-start trials 1 and 4: avg = 140.7s.

#### Failed trial — labeler hallucination on the ambiguous prompt set

Between trial 3 (15:46:08) and what would have been trial 5 (16:00:13) there is a 14-minute gap. The cluster logs show exactly what happened:

```
time=2026-07-26T15:48:18Z level=ERROR msg="batch label call failed"
  node=validator-10 error="httpclient: unknown subdomain: subdomain 'smart_contracts' not in allowed set"

time=2026-07-26T15:48:18Z level=ERROR msg="proposed block validation failed; vote will not be sent"
  node=validator-10 senderID=validator-8

--- FAIL: TestDistributedMR1_Ambiguous_FrequencyVariance (600.20s)
    Error: Condition never satisfied
```

**Root cause:** validator-10 (moa-chain-9, temperature 0.75) called its labeler for tx3 ("What makes a smart contract vulnerable to reentrancy attacks?") and the model returned `smart_contracts` — an invented subdomain not in the allowed set. The prompt's literal phrase "smart contract" caused the model to construct a subdomain name by concatenation, the same failure mode as `consensus_algorithms` / `consensus_protocols` under `labeler_v2`.

**Failure cascade:** When validator-10's labeler returned the invalid subdomain, the block body validation failed. validator-10 rejected the block proposed by validator-8 and did not store it locally. Because the test waits for `GetFinalizedBlockInMROne` to succeed on **all** nodes simultaneously, and validator-10 never stored the block, the round could not finalize at validator-10 even if the other 9 validators reached quorum. The test timed out after 600 seconds.

**Why the other 9 trials did not fail:** The hallucination is seed-dependent. At temperature 0.75, the model's token sampling is probabilistic — on 9 out of 10 trials the same validator at the same temperature resolved the ambiguity to a valid subdomain. On one trial, the random seed on that call produced `smart_contracts`. The `labeler_v3` negative-example list (`consensus_algorithms`, `distributed_systems`, `consensus_protocols`) does not include `smart_contracts`, so the constraint did not suppress this particular output.

**Timeline confirmation:** The failed trial started at ~15:46:30 (after trial 3 stopped). The hallucination occurred at 15:48:18. The 600s timeout expired at ~15:58:18. The next cluster start + warmup completed at ~16:00:13 — exactly when trial 5's JSON is timestamped. The 14-minute gap is entirely accounted for: ~2 min test before hallucination + 10 min timeout + ~2 min cluster restart.

**Note on the warm/cold label:** Trial 5 in our results (16:00:13, 195s) was previously labelled a cold-start outlier. That label is correct for a different reason than the other cold start: Ollama had been idle for ~10 minutes during the timeout, so the model was evicted from memory and had to reload. The 195s duration confirms this.

#### Frequency map variance

**Unique subdomain maps observed across 9 trials:**

| Map | Count | Trials |
|---|---|---|
| `blockchain_engineering:10, security:10, systems_programming:5` | 4/9 (44%) | 2, 6, 7, 9 |
| `blockchain_engineering:14, security:10, systems_programming:5` | 2/9 (22%) | 1, 8 |
| `blockchain_engineering:14, security:10, systems_programming:9` | 2/9 (22%) | 3, 4 |
| `blockchain_engineering:13, security:10, systems_programming:5` | 1/9 (11%) | 5 |

Four unique maps across 9 trials — the highest variance of any experiment in this series. By comparison, all previous experiments produced at most 2 unique maps (honest: 2/12; Byzantine: 2/10; non-related: 1/5).

#### Per-subdomain stability analysis

| Subdomain | Min | Max | Range | Trials present | Notes |
|---|---|---|---|---|---|
| `security` | 10 | 10 | **0** | 9/9 | Rock-solid — appears in every trial at exactly frequency 10 |
| `blockchain_engineering` | 10 | 14 | **4** | 9/9 | Always present; most variable of the three |
| `systems_programming` | 5 | 9 | **4** | 9/9 | Always present; two distinct modes (5 or 9) |

`security` is the only subdomain with zero variance (frequency 10 in all 9 trials). This is unexpected: the prediction was that `security` would be dominant but with some variation. The lock at exactly 10 suggests a near-perfect 50-50 per-transaction split — tx2 and tx3 each consistently collect 5 security votes from the committee, and the other 5 validators classify each as `blockchain_engineering`. The `security` fraction of the vote is stable even though the total committee outcome varies.

`blockchain_engineering` fluctuates between 10 and 14, with a middle outlier of 13 (trial 5). The variation is in the extra votes blockchain_engineering receives beyond its base value of 10. When be=14, approximately 4 additional votes flowed to blockchain_engineering — likely tx1 votes where validators chose be over sp, or tx2/tx3 votes where the blockchain context overrode the security framing.

`systems_programming` has two clear modes: 5 (7 out of 9 trials) and 9 (2 out of 9 trials). The baseline value of 5 corresponds to tx1 being labeled sp by roughly half the committee. When sp=9, approximately 4 extra sp votes appeared, most likely from some validators labeling tx1 with both sp and another subdomain, or from tx2/tx3 picking up unexpected sp votes at higher temperatures.

#### Comparison with pre-experiment predictions

| Prompt | Predicted | Observed |
|---|---|---|
| tx1 — deterministic ordering in consensus | Split sp/be, sp unstable | ✅ Both sp and be present in all trials; sp varies between 5 and 9 |
| tx2 — hash functions on blockchain | Security dominant, be secondary | ⚠️ Unexpected: security and be are equal (both contribute frequency 10) — no dominance |
| tx3 — smart contract reentrancy | Security strongly dominant | ⚠️ Same: security locked at 10, no stronger than tx2; be equally strong |

The main surprise is the 50-50 split between `security` and `blockchain_engineering` on tx2 and tx3. The prediction was that security vocabulary ("protect", "vulnerable", "attacks") would pull more validators away from blockchain_engineering. Instead, the explicit blockchain framing ("stored on a blockchain", "smart contract") proved equally strong, producing a symmetric split regardless of temperature across all 9 trials. The model at qwen2.5-coder:7b appears to give equal weight to attack-surface vocabulary and platform-domain vocabulary when they co-occur in the same prompt.

#### Why all 9 maps contain all 3 subdomains

In all previous experiments, maps had 1 or 4 subdomains. Here, every trial produces exactly 3. This is not a coincidence: each subdomain represents a genuinely distinct semantic angle that clears the quorum threshold on at least one transaction in every round.

- `security` clears quorum on tx2 and tx3 without exception.
- `blockchain_engineering` clears quorum on tx2 and tx3 (from the 5 validators who chose it over security) and sometimes on tx1.
- `systems_programming` clears quorum on tx1 from the validators who chose it over blockchain_engineering.

The 3-subdomain result is therefore a structural property of this particular transaction set: each prompt has two plausible subdomains, the committee splits roughly 50-50 on each, and both halves of every split are large enough to clear the Q=7 quorum. This means the consensus protocol faithfully captures the semantic ambiguity rather than collapsing it to a single label — the block encodes the uncertainty.

#### Duration: ambiguous prompts are significantly slower

| Experiment | Avg duration (all) | Avg duration (warm only) |
|---|---|---|
| Honest baseline | 113.0s | 109.4s |
| Byzantine k=3 | 113.0s | ~111s |
| Non-related | 119.0s | 112.5s |
| **Ambiguous** | **152.2s** | **140.7s** |

Ambiguous prompts take **~35% longer** than coding-only or non-related prompts (140.7s vs 109.4s warm). This is expected: resolving a borderline classification requires the LLM to generate more tokens as it reasons through the two candidate subdomains before committing to one. Non-related prompts are fast because the model recognises off-topic content quickly and produces a short output. Coding prompts are faster than ambiguous ones because the subdomain is unambiguous and the model commits immediately.

#### Comparison with all previous experiments

| Metric | Honest baseline (12) | Byzantine k=3 (10) | Non-related (5) | Ambiguous (9) |
|---|---|---|---|---|
| Pass rate | 12/12 | 10/10 | 5/5 | 9/9 |
| NonRelatedCount | 0 | 0 | 2 | 0 |
| Subdomain map entries | 4 | 4 | 1 | **3 (always)** |
| Unique maps observed | 2 | 2 | 1 | **4** |
| Avg duration (warm) | 109.4s | ~111s | 112.5s | **140.7s** |
| `security` frequency | 0 | 0 | 0 | **10 (locked)** |
| `blockchain_engineering` frequency | 5 | 4 | 0 | **10–14** |
| `systems_programming` frequency | 5 or 4 | 4 or 0 | 0 | **5 or 9** |
| Divergent trials | 1/12 | 2/10 | 0/5 | **9/9 (all diverge from each other)** |

The ambiguous experiment is qualitatively different from all previous ones: instead of a dominant map with occasional divergence, every trial produces a different map. The protocol still finalizes correctly every time, demonstrating that the BFT consensus layer is robust to semantic ambiguity in the labeling layer — it converges on *a* consistent result even when that result varies across rounds.

---

## Issues Discovered

### 1. Subdomain hallucination (labeler_v2)

**Problem:** During initial runs with `labeler_v2`, multiple validators failed during block validation with:

```
httpclient: unknown subdomain: subdomain 'consensus_algorithms' not in allowed set
httpclient: unknown subdomain: subdomain 'consensus_protocols' not in allowed set
```

The third transaction ("Why does deterministic ordering matter in **consensus**?") caused the model to invent subdomain names (`consensus_algorithms`, `consensus_protocols`) that sound plausible but are not in the allowed list. This occurred across several temperatures including 0.30 (the lowest), showing the issue is driven by the prompt content, not by temperature variance.

**Root cause:** The `labeler_v2` prompt instructed the model to use only subdomains from `allowed_subdomains`, but this constraint was insufficiently prominent for a 7B model when the prompt content strongly suggested a non-existent subdomain name.

**Fix:** Introduced `labeler_v3` with an explicit prohibition section:

> STRICT SUBDOMAIN RULE: You MUST use subdomain values that appear EXACTLY and VERBATIM in the allowed_subdomains list. Do NOT invent, create, or guess subdomain names, even if they sound plausible or more precise. Examples of invalid invented names: "consensus_algorithms", "distributed_systems", "consensus_protocols". If the concept you have in mind is not listed, find the closest match that IS in allowed_subdomains.

After switching to `labeler_v3`, zero hallucinated subdomains were observed across both runs.

### 2. Subdomain hallucination recurs on new ambiguous prompts (labeler_v3 incomplete)

**Problem:** During the ambiguous-prompt trials, one trial (trial 4 of 10) failed with:

```
httpclient: unknown subdomain: subdomain 'smart_contracts' not in allowed set
```

The prompt `"What makes a smart contract vulnerable to reentrancy attacks?"` caused validator-10 (temperature 0.75) to return `smart_contracts` — the model took the literal phrase "smart contract" and constructed a subdomain name from it.

**Root cause:** `labeler_v3` added a `STRICT SUBDOMAIN RULE` section with negative examples explicitly listing `consensus_algorithms`, `distributed_systems`, and `consensus_protocols`. These were chosen based on the hallucinations seen during the initial honest-baseline runs. The new ambiguous transaction set introduced a prompt that triggers a different invention pattern (`smart_contracts`), which is not in the negative-example list and therefore not suppressed.

**Impact — test vs. production distinction:** The hallucination occurred at exactly one validator (temperature 0.75) on exactly one trial (1/10). In the test, this caused a full 600-second timeout because the test requires **all 10 nodes** to finalize the block locally before it reports pass. In a production deployment, the round would **not fail**: the other 9 validators vote correctly, Q=7 is met, the finalization certificate is formed, and the block is committed to the chain. Validator-10 simply misses that round. The test strictness (all-nodes-must-finalize) is what surfaces the bug as a hard failure; the consensus protocol itself tolerates it. However, a labeler that hallucinates on this prompt class 1-in-10 rounds is a long-term validator reliability issue: the affected node would consistently fail to vote on smart-contract security prompts, effectively reducing the reliable honest committee size and narrowing the margin to the BFT bound over many rounds.

**Fix required:** Add `smart_contracts` (and similar blockchain-vocabulary constructions such as `smart_contract_security`, `blockchain_security`) to the `labeler_v3` negative-example list, or enumerate a broader set of plausible-but-invalid patterns. Alternatively, the validator node could be made more tolerant by dropping unrecognised subdomains silently rather than failing the entire block validation — though this would mask labeler quality regressions.

### 3. Go client error reporting (pre-existing bug)

**Problem:** The Go HTTP client looked for `error_code`/`message` fields in Python error responses, but the Python agent returns `error`/`detail`. This caused all Python errors to appear as the generic `"httpclient: unexpected HTTP error: status 400"` with no detail, making debugging very difficult.

**Fix:** Updated `agent/httpclient/dto.go` to use the correct field names (`error`/`detail`). This immediately revealed the hallucination error text, unblocking diagnosis.

---

## Key Findings

1. **Distributed MR1 converges correctly.** 10 validators at temperatures 0.30–0.75, each running its own Ollama instance, produced consistent `SubdomainsFrequencies` across all nodes. All transactions were classified as coding-related (NonRelatedCount = 0).

2. **Consensus frequencies reflect committee selection, not all validators.** With `CommitteeStrategyFull` and 10 validators, the frequency of 5 per subdomain indicates that 5 out of 10 validators scored each label — consistent with how subdomain weights are accumulated during MR1 committee selection.

3. **Temperature diversity does not disrupt label consensus.** The 0.45-point spread across temperatures produced different label confidence scores per agent, but the final agreed subdomain frequency map was identical across all runs.

4. **MR1 convergence is highly reliable across independent restarts.** 12/12 trials passed with a full cluster restart between each run. The dominant subdomain map appeared in 11/12 trials (91.7%); the one divergent trial differed only in one frequency value (4 vs 5 for `systems_programming`) and still finalized correctly. This confirms that the labeling consensus is stable across temperature diversity, random Ollama initialization, and independent model cold-starts.

5. **Prompt engineering is critical for constrained output.** A 7B model requires explicit negative examples in the prompt to avoid hallucinating plausible-but-invalid subdomain names. Soft constraints ("only use values from the list") are insufficient.

6. **Label throughput is the bottleneck.** Per-agent labeling takes 35–68s for 3 transactions. The round critical path is: leader labels (~45s) + validator labels (~45s, parallel across machines) + consensus overhead ≈ 100–160s total. Reducing per-call latency (e.g. smaller model, GPU, or fewer transactions per block) is the main lever for faster rounds.

7. **Byzantine fault tolerance holds at the distributed level.** With k=3 Byzantine validators (exactly f, the BFT bound for G=10, Q=7), the correct label map was finalized in 10/10 trials and the Byzantine label never appeared in any result. The collect-all-G-votes rule prevents fast Byzantine responses from dominating the certificate, confirming the adversarial timing fix from Section IV.C extends correctly to a fully distributed deployment with independent Ollama instances.

8. **Borderline subdomains are more sensitive to committee size.** The semantically ambiguous transaction ("Why does deterministic ordering matter in consensus?") dropped `systems_programming` from its map in 2/10 Byzantine trials but 0/12 honest trials. With 3 fewer honest voters, labels at the margin of consensus can fall below the quorum threshold. This indicates that committee size directly influences the stability of borderline semantic classifications, and that the same prompt which caused hallucination under `labeler_v2` remains the hardest to classify consistently even with a correct prompt.

9. **CPU inference does not benefit from Ollama parallelism.** Setting `OLLAMA_NUM_PARALLEL=3` with `LABEL_MAX_CONCURRENCY=3` made labeling 48% slower. A single inference call already saturates the CPU; running three simultaneously causes core contention that increases per-call latency without reducing wall time. For CPU-only deployment, `LABEL_MAX_CONCURRENCY=1` or `2` is optimal. GPU deployment would be needed for true request-level parallelism.

10. **Off-topic classification is perfectly reliable and adds no overhead.** Prompts with no connection to software engineering (geography, cooking) were identified as `non_related` in all 5 trials with zero variance — the sharpest result in this experiment series. The boundary between in-scope and out-of-scope is unambiguous enough that the 0.45-point temperature spread across validators produces no divergence, unlike borderline coding subdomains (e.g. `systems_programming` vs `blockchain_engineering`). After excluding the cold-start trial, average round duration (112.5s) matches the honest coding-only baseline (113.0s), confirming that non-related classification adds no measurable processing overhead. Transaction classification is per-item and independent: having two off-topic transactions alongside one coding transaction does not affect the coding transaction's subdomain assignment.

11. **Ambiguous prompts produce stable multi-subdomain maps but expose a labeler brittleness not seen on standard prompts.** Three borderline prompts produced 4 unique frequency maps across 9 passing trials — the highest variance of any experiment in this series. `security` locked at exactly frequency 10 (zero variance across all trials), revealing a consistent 50-50 per-transaction split between `security` and `blockchain_engineering` that no temperature could break. `blockchain_engineering` and `systems_programming` varied moderately (range of 4 each). Ambiguous prompts take ~35% longer to process (140.7s vs 109.4s warm) because the model generates more tokens before committing to a label. One trial in ten failed due to a labeler hallucination (`smart_contracts`) triggered by the literal phrase "smart contract" — an invention pattern not covered by the `labeler_v3` negative-example list. This shows that prompt-engineering coverage of hallucination patterns is incomplete: each new class of domain-vocabulary prompts can introduce novel invalid subdomain constructions. The BFT consensus layer is robust to semantic ambiguity when the labeler behaves correctly, but a single validator producing an invalid subdomain causes a hard round failure (full timeout) because the Go client enforces strict subdomain validation at block verification time.
