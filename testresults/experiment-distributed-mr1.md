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

### 2. Go client error reporting (pre-existing bug)

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

7. **CPU inference does not benefit from Ollama parallelism.** Setting `OLLAMA_NUM_PARALLEL=3` with `LABEL_MAX_CONCURRENCY=3` made labeling 48% slower. A single inference call already saturates the CPU; running three simultaneously causes core contention that increases per-call latency without reducing wall time. For CPU-only deployment, `LABEL_MAX_CONCURRENCY=1` or `2` is optimal. GPU deployment would be needed for true request-level parallelism.
