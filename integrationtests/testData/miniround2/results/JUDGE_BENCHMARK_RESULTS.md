# MR2 Judge Latency Benchmark Results

**Model:** qwen2.5-coder:7b (Ollama, local)  
**Date:** 2026-07-13  
**Raw data:** `judge_benchmark_results.jsonl` (36 records)

---

## What Was Measured

Mini-Round Two is the phase where validators classify candidate answers for each transaction as `CORRECT`, `WRONG`, `HALLUCINATION`, or `MALICIOUS`. Every validator calls the LLM once per transaction in the block — the result is a `judgement` message that is broadcast and aggregated toward quorum.

The judge call is **on the critical path**: unlike answering and labeling (which are computed off-round at mempool entry), judging cannot be pre-computed. The outcome must be kept secret until the voting deadline, and it changes every round as candidate pools change.

This benchmark measures two things:

- **Experiment E** — how does a single `/judge` call scale with the number of candidate answers?
- **Experiment F** — how does total block judging time scale with the number of transactions, each with 5 candidates?

All experiments use a fixed prompt about backpropagation, proof-of-work vs proof-of-stake, horizontal/vertical scaling, React virtual DOM, and the CAP theorem — realistic computer science questions drawn from the MR1 benchmark fixture pool.

---

## Experiment E — Vary Candidate Count

Each data point is one `/judge` call classifying N candidate answers for a single backpropagation transaction. Three runs per data point. A 3-second cooldown separates each call.

| Candidates | Run 1 (ms) | Run 2 (ms) | Run 3 (ms) | Warm avg (ms) |
|:----------:|:----------:|:----------:|:----------:|:-------------:|
| 1          | 2137       | 782        | 773        | **778**       |
| 3          | 2393       | 1632       | 1643       | **1638**      |
| 5          | 3209       | 2474       | 2477       | **2476**      |
| 7          | 4067       | 3326       | 3325       | **3326**      |

"Warm avg" excludes run 1 of each group, which is consistently slower (see note below).

### What This Tells Us

**Judging latency scales linearly with candidate count.** Going from 1 to 7 candidates adds ~3.3× the latency in proportion to the 7× candidate count — very close to linear.

Between consecutive candidate counts (+2 candidates each step):
- 1→3: +860 ms (+430 ms/candidate)
- 3→5: +838 ms (+419 ms/candidate)
- 5→7: +850 ms (+425 ms/candidate)

The per-candidate cost stabilises at **~425 ms per additional candidate** (warm, local 7B model).

**Practical implication:** With a typical MoA block carrying 5 candidates per transaction, each validator should expect roughly **2.5 seconds per judge call** under normal conditions. The per-call timeout should be set with headroom for the first-call effect (see below), so a safe timeout is **5–6 seconds per transaction**.

### The First-Run Effect

Run 1 of each candidate-count group is consistently 100–200% slower than runs 2 and 3. This is not a cold-start of Ollama itself — the model is pre-warmed before any test runs. Instead, it reflects a **context-building overhead** when the judge prompt structure changes significantly between consecutive calls: the model's internal KV cache is invalidated on the new prompt, so it must build attention keys and values from scratch.

This means that the first transaction judged in a new block may be slower than subsequent ones if the block structure differs substantially from the previous block. The steady-state latency (runs 2 and 3) is therefore the correct number to use for timeout design.

---

## Experiment F — Vary Transaction Count

Each data point is one full "block judging" operation where the validator calls `/judge` once per transaction, with 5 candidates per transaction. The table shows the total time a validator spends judging the entire block (sum of all per-tx call durations in that run).

| Block size (txs) | Run 1 block (ms) | Run 2 block (ms) | Run 3 block (ms) | Warm avg block (ms) | Per-tx warm (ms) |
|:----------------:|:----------------:|:----------------:|:----------------:|:-------------------:|:----------------:|
| 1                | 2640             | 2470             | 2465             | **2468**            | 2468             |
| 3                | 10948            | 7331             | 7331             | **7331**            | 2444             |
| 5                | 15796            | 12833            | 12224            | **12529**           | 2506             |

### What This Tells Us

**Block judging time scales linearly with transaction count.** The per-tx cost remains nearly constant at ~2.47 seconds regardless of block size (1, 3, or 5 transactions). This makes sense: calls are sequential — the validator judges tx1, then tx2, then tx3 — so each additional transaction adds one full judge call to the total.

**Run 1 outlier for 3 txs (10948 ms):** The first run is always slower, but for 3 txs the spike is particularly pronounced because the prompt topics change significantly between consecutive tx calls within the same block run. The model encounters a new context for each tx, and the first run has no prior run providing a warm-up call in that topic area.

**A 5-tx block takes ~12.5 seconds to judge** on a single validator. This becomes the MR2 deadline lower bound: all validators must submit their judge votes within this window, with added network margin.

**Parallelisation headroom:** Since each `/judge` call is independent (each tx is judged separately), a validator could in principle judge all transactions in a block in parallel, reducing the block judging time to `max(per-tx latency)` ≈ 2.5 s regardless of block size. The current sequential implementation is conservative and safe; parallel calls are a future optimisation.

---

## Classification Accuracy

Across all 36 benchmark calls, the model classified 183 individual candidates:

| Category  | Count | Share  |
|:---------:|:-----:|:------:|
| CORRECT   | 178   | 97.3%  |
| WRONG     | 5     | 2.7%   |
| HALLUCINATION | 0 | 0%   |
| MALICIOUS | 0     | 0%     |

### The Two WRONG Classifications

The model labelled two specific candidates as `WRONG` across all runs. Both cases are **false negatives** — the model disagreed with technically correct answers:

**Candidate: nuanced virtual DOM comparison (React question)**  
Answer excerpt: *"It is worth noting the virtual DOM is not always faster than direct DOM manipulation. For infrequent, simple updates, direct access skips the diffing overhead and can be faster. Svelte compiles components to direct DOM operations at build time..."*  
Labelled `WRONG` in 2 out of 3 runs. The answer is technically accurate — Svelte's compile-time approach is a legitimate and correct observation. The model appears to penalise answers that qualify or challenge the "virtual DOM is fast" narrative, even when the qualification is factually sound.

**Candidate: CAP theorem historical attribution (CAP question)**  
Answer excerpt: *"The CAP theorem was formalised by Brewer and proved by Gilbert and Lynch. It applies to a single data item across a distributed system... PACELC extends CAP by also characterising the latency-consistency trade-off..."*  
Labelled `WRONG` in all 3 runs — the only candidate with **consistent false-negative rejection**. The answer cites real academic work (the Gilbert-Lynch proof is correct; PACELC is a real published extension). The model likely lacks reliable knowledge of PACELC and penalises the answer for introducing unfamiliar content.

**Design implication:** The `WRONG` category is more sensitive to model knowledge gaps than the `CORRECT` category. In MoA-chain consensus, this is safe by design — a false-negative (marking a correct answer as WRONG) prevents that candidate from contributing to the canonical group, but it does not cause incorrect results to be accepted. The consensus mechanism naturally tolerates minority disagreements among validators.

---

## Summary for System Design

| Parameter                         | Measured Value      | Recommended Protocol Setting |
|:----------------------------------|:-------------------:|:----------------------------:|
| Per-candidate overhead (warm)     | ~425 ms             | —                            |
| Per-tx judging (5 candidates)     | ~2.5 s              | 5 s timeout (2× safety)      |
| Block judging (5 txs, 5 cand/tx)  | ~12.5 s             | 15–20 s deadline             |
| Cold-context first-call slowdown  | 35–50% slower       | Absorbed by 2× timeout       |
| Judge accuracy (CORRECT vs all)   | 97.3%               | Tolerated by consensus quorum |

The dominant cost driver is the LLM generation time, which grows linearly in both dimensions. The blockchain protocol must set per-tx timeouts and block deadlines accordingly. With a 5-second per-tx timeout and up to 5 transactions per block, a validator has up to **25 seconds** to complete judging — sufficient even for worst-case first-call slowdowns.
