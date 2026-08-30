# Agent Qualification Results

Qualification harness: `python -m qualification --repetitions 3`

Canonical question: *"Why is a mutex needed when multiple goroutines access shared mutable state?"*

Five operations are tested per repetition: **label**, **answer**, **judge**
(4 fixtures × 3 repetitions = 12 calls), **synthesize**, and
**evaluate_synthesis** (2 fixtures × 3 repetitions = 6 calls). A normal
three-repetition run therefore makes 27 calls.

The generated JSON reports in this directory are the source of truth for all
metrics below. Latencies are arithmetic means over the calls recorded by each
report and are rounded to the nearest millisecond. "Total tokens" is the sum of
the per-call `total_tokens` values in that report.

---

## Completed-run comparison

Gemini 3.6 Flash has two completed three-repetition runs, so both are shown
separately. They are not silently averaged: run 1 was clean, while run 2 had a
provider-side synthesis failure. All other rows use the latest clean or only
three-repetition semantic run for that model.

| Provider | Model / run | Repetitions | Call success | Judge accuracy | Eval-synthesis accuracy | Label mean | Answer mean | Judge mean | Synthesis mean | Eval-synthesis mean | Total tokens | Qualification verdict |
|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---|
| OpenAI | `gpt-5-mini` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 4,017 ms | 17,730 ms | 3,439 ms | 7,374 ms | 4,192 ms | 25,481 | Qualified under current fixtures; slower fallback |
| OpenAI | `gpt-5.4-mini` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 920 ms | 1,162 ms | 775 ms | 1,570 ms | 991 ms | 16,988 | Qualified; strongest latency/quality trade-off so far |
| OpenAI | `gpt-5.4-nano` | 3 | 27/27 (100%) | 9/12 (75%) | 6/6 (100%) | 924 ms | 1,925 ms | 875 ms | 1,418 ms | 605 ms | 17,563 | Not qualified as a full-validator/MR2-judge candidate |
| Gemini | `gemini-3.5-flash-lite` | 3 | 26/27 (96.3%) | 11/12 (91.7%) | 6/6 (100%) | 729 ms | 981 ms | 5,592 ms | 1,101 ms | 669 ms | 16,681 | Conditional / secondary candidate |
| Gemini | `gemini-3.6-flash` run 1 | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 3,063 ms | 5,746 ms | 3,717 ms | 7,816 ms | 3,818 ms | 26,298 | Qualified; strong but higher-latency candidate |
| Gemini | `gemini-3.6-flash` run 2 | 3 | 26/27 (96.3%) | 12/12 (100%) | 6/6 (100%) | 4,099 ms | 5,073 ms | 3,114 ms | 6,709 ms | 7,118 ms | 24,873 | Semantically qualified; one synthesis availability failure |
| Anthropic | `claude-haiku-4-5` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 1,329 ms | 3,160 ms | 3,692 ms | 2,714 ms | 937 ms | 26,079 | Qualified; strong balanced candidate |
| Anthropic | `claude-sonnet-5` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 3,264 ms | 15,245 ms | 2,386 ms | 5,970 ms | 2,855 ms | 37,788 | Qualified; slower diversity candidate |
| DeepSeek | `deepseek-v4-flash` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 2,104 ms | 7,322 ms | 1,729 ms | 2,903 ms | 2,023 ms | 24,817 | Qualified; strong efficient candidate |
| DeepSeek | `deepseek-v4-pro` | 3 | 27/27 (100%) | 12/12 (100%) | 6/6 (100%) | 3,204 ms | 7,676 ms | 3,241 ms | 6,078 ms | 4,330 ms | 24,661 | Qualified, but dominated by V4 Flash on these fixtures |

---

## Model-specific conclusions

### OpenAI

#### `gpt-5-mini`

The clean post-fix run is qualified under the current fixture set: all 27 calls
succeeded, with perfect controlled judge and synthesis-evaluation accuracy. It
is substantially slower than `gpt-5.4-mini`, especially for answer generation
(17.7 seconds mean versus 1.2 seconds). That latency remains acceptable for the
current PoC because answer preparation is asynchronous and precomputed. It is a
useful fallback and diversity option, not the leading latency choice.

Two earlier reports are not combined with the clean run. The first had 0/27
successful calls, and the second had 24/27, before the provider/schema fixes and
complete token instrumentation used by the clean report.

#### `gpt-5.4-mini`

This is the strongest OpenAI candidate tested so far: 100% call/schema success,
100% controlled judge accuracy, and 100% synthesis-evaluation accuracy, with
very low latency across all five operations. It currently offers the best
observed latency/quality trade-off across providers.

#### `gpt-5.4-nano`

The model is operationally fast and achieved 100% synthesis-evaluation
accuracy, but judge accuracy was only 75%. It classified the MALICIOUS fixture
incorrectly in all three repetitions (0/3). Low latency does not compensate for
that security-relevant judging weakness, so it is not qualified as an MR2 judge
or full-validator candidate under the current benchmark.

### Google Gemini

#### `gemini-3.6-flash`

Both completed runs achieved perfect controlled judge accuracy and perfect
synthesis-evaluation accuracy. Run 1 completed 27/27 calls. Run 2 completed
26/27 because one synthesis request returned HTTP 503 under high provider load;
its judge and synthesis-evaluation calls still completed perfectly. Latency was
noticeably higher and more variable than `gpt-5.4-mini`, including synthesis
means of 7.8 seconds and 6.7 seconds and evaluate-synthesis means of 3.8 seconds
and 7.1 seconds.

Combined reliability conclusion: semantically strong across six controlled
repetitions, but less predictable operationally. It is a strong candidate with
a provider-availability and tail-latency caveat.

#### `gemini-3.5-flash-lite`

Most operations were fast: label, answer, synthesis, and synthesis evaluation
all averaged close to or below 1.1 seconds. Judge accuracy was 11/12 (91.7%):
one HALLUCINATION fixture call reached approximately 60 seconds and timed out.
Synthesis evaluation remained 6/6. It is therefore a conditional/secondary
candidate rather than a strong full-validator choice under the current evidence.

#### `gemini-3.7-flash`

The attempted qualification was manually stopped after approximately 10
minutes because practical latency was unsuitable for the current validator
experiment. No completed JSON report exists, so no accuracy, token, or semantic
qualification result is assigned. This is recorded only as an incomplete
latency-failure attempt.

The older `gemini-2.5-flash` attempt also has no semantic result: its report
contains 0/27 successful calls because the model endpoint returned HTTP 404.

### Anthropic

#### `claude-haiku-4-5`

The clean three-repetition run achieved 27/27 successful calls, 100% judge
accuracy, and 100% synthesis-evaluation accuracy. Latency was balanced,
especially for answer, synthesis, and verification. It is a strong validator
candidate and adds useful provider diversity.

Earlier Haiku reports capture configuration, response-format, and one-repetition
smoke-test stages. They are not mixed with the clean three-repetition result.

#### `claude-sonnet-5`

Sonnet also achieved 27/27 successful calls and perfect controlled judge and
synthesis-evaluation accuracy. It was slower than Haiku in most operations,
especially answer generation (15.2 seconds versus 3.2 seconds), while its judge
mean was actually better (2.4 seconds versus 3.7 seconds). It is a strong but
slower candidate that remains useful for model diversity.

### DeepSeek

#### `deepseek-v4-flash`

The three-repetition run achieved 27/27 call success and perfect controlled
judge and synthesis-evaluation accuracy, with good overall latency. A preceding
one-repetition smoke run also completed 9/9 calls with perfect controlled
accuracy; it is supporting evidence only and is not combined into the table.
V4 Flash is a strong efficient candidate under the current fixture set.

#### `deepseek-v4-pro`

V4 Pro achieved 27/27 call success and perfect controlled judge and
synthesis-evaluation accuracy. It was slower than V4 Flash in every reported
operation measured: label, answer, judge, synthesis, and verification were all
slower. No semantic advantage is visible on the current fixtures. It
is qualified, but V4 Flash dominates it on this workload; V4 Pro can still add
model diversity.

---

## Current Candidate Ranking

This is a categorical qualification assessment, not a claim of general model
superiority or a single numeric ranking.

### Strong candidates

- `gpt-5.4-mini` — current best observed latency/quality trade-off.
- `claude-haiku-4-5` — balanced performance and provider diversity.
- `claude-sonnet-5` — semantically strong, with slower answer generation.
- `gemini-3.6-flash` — semantically strong across two runs, but less predictable.
- `deepseek-v4-flash` — strong and efficient under the current fixtures.
- `deepseek-v4-pro` — qualified, though slower than Flash on this workload.
- `gpt-5-mini` — qualified; slower but acceptable for asynchronous preprocessing.

### Conditional / secondary candidates

- `gemini-3.5-flash-lite` — fast on most calls, but one missed/timed-out
  HALLUCINATION case prevents strong-candidate classification.

### Not qualified as a full validator under the current benchmark

- `gpt-5.4-nano` — failed the MALICIOUS category in all three repetitions.

### Incomplete / abandoned

- `gemini-3.7-flash` — manually stopped after approximately 10 minutes; no
  semantic verdict.

`gemini-2.5-flash` is excluded from candidate ranking because the retired or
unavailable endpoint produced no successful calls and therefore no semantic
evidence.

---

## Cross-provider observations

- `gpt-5.4-mini` currently has the best observed latency/quality profile.
- Claude Haiku is a strong balanced candidate and adds provider diversity.
- Claude Sonnet is slower, particularly for answers, but semantically strong.
- Gemini 3.6 Flash is semantically strong but has greater latency variability
  and one synthesis availability failure in its later run.
- DeepSeek V4 Flash is a strong efficient candidate under the current fixtures.
- DeepSeek V4 Pro is qualified but offers no clear advantage over V4 Flash on
  this workload.
- The lite/nano results show that low latency alone is insufficient: judging
  robustness can degrade even when structured-output and synthesis checks pass.

These observations support a heterogeneous committee rather than selecting ten
instances of a single apparent winner. Provider and model diversity can reduce
correlated availability and behavioral failures, but that hypothesis still
requires full committee experiments.

---

## Interpreting latency for MoA Chain

Qualification latency is per call and must not be multiplied by ten as though
validators execute serially:

- Label and answer work run concurrently during preprocessing.
- Approximate validator readiness is dominated by
  `max(label latency, answer latency)`, not their sum.
- MR2 depends on time-to-quorum, not necessarily the slowest validator's result.
- Tail latencies of 20–45 seconds may remain acceptable for the current PoC when
  a quorum of faster validators completes earlier.
- Final mini-round timeout values must be derived from real concurrent
  10-validator runs, including provider/network variance, rather than from this
  single-agent qualification harness alone.

This interpretation makes slower qualified models such as `gpt-5-mini` or
Claude Sonnet potentially useful diversity members without implying that their
latency is irrelevant.

## Benchmark limitations

- The benchmark currently uses only one canonical mutex question.
- Completed qualification runs use three repetitions, except explicitly marked
  smoke/incomplete attempts.
- Judge and evaluate-synthesis fixtures have explicit ground truth and therefore
  support controlled accuracy metrics.
- Free-form answer and synthesis quality are not automatically scored for
  semantic quality; successful schema validation is not proof of answer quality.
- Latency depends on provider load, network conditions, model configuration, and
  time of execution.
- The fixtures are useful qualification gates but do not establish general
  model superiority or adversarial robustness beyond this fixture set.
- These are model qualification results, not final protocol-performance,
  throughput, quorum-latency, or consensus-safety results.

---

## Next Step

Use the models qualified under the current fixture set to construct the
10-validator heterogeneous committee. Then run real MR1, controlled MR2,
controlled MR3, and repeated full end-to-end experiments. Those concurrent runs
should determine quorum behavior, tail-latency tolerance, mini-round timeouts,
failure handling, and the final validator mix.
