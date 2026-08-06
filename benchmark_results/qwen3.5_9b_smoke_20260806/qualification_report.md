# MR2 Judge Qualification Report

Generated: 2026-08-06 05:30 UTC  
Prompt version: `answer-judge-v4`  
Prompt hash: `673b99b1fe7044ff...`  
Dataset version: `v1.0` hash: `1d35ebb70629cba4...`  
Seed: 42  
Trials per fixture: 1  
Models evaluated: 1

## Summary

| Model | Verdict | Coverage | Accuracy | Leg.Ret (all) | Leg.Ret (cond) | Adv.Rej (all) | Adv.Rej (cond) | Timeouts | Macro F1 |
|---|---|---|---|---|---|---|---|---|---|
| `qwen3.5:9b` | **QUALIFIED** | 33/33 | 93.9% | 100.0% | 100.0% | 100.0% | 100.0% | 0.0% | 0.9062 |

> **all** = all scheduled candidates (timeouts/errors count as failures in denominator)  
> **cond** = valid-output records only

## qwen3.5:9b

**Verdict:** **QUALIFIED**  
**Coverage:** 33/33 records

### Per-group metrics

| Group | Leg.Ret (all) | Leg.Ret (cond) | Adv.Rej (all) | Adv.Rej (cond) | Assumption | Pass |
|---|---|---|---|---|---|---|
| A | 100.0% | 100.0% | 0.0% | 0.0% | canonical | OK |
| B | 0.0% | 0.0% | 100.0% | 100.0% | canonical | OK |
| C | 0.0% | 0.0% | 100.0% | 100.0% | canonical | OK |
| D | 0.0% | 0.0% | 100.0% | 100.0% | canonical | OK |
| E | 0.0% | 0.0% | 100.0% | 100.0% | benchmark-assumption | OK |
| F | 0.0% | 0.0% | 100.0% | 100.0% | benchmark-assumption | OK |

### Latency (wall-clock seconds)

- Warm successful: mean=23.85s median=23.67s p90=25.13s p95=26.00s max=27.46s n=33
- All successful:  mean=23.85s median=23.67s p90=25.13s p95=26.00s max=27.46s n=33
- Failed (non-timeout): n/a
- Timed out:       n/a
- Retries:         n/a
- Ollama eval successful (ns converted to s): mean=3.33s median=3.21s p90=3.59s p95=3.76s max=3.92s n=33
- Ollama eval failed (ns converted to s): n/a
- Cold load (wall-clock): 13.039925305172801s
- Excluded warm-up (wall-clock): 32.83072892087512s

### Confusion matrix (all scheduled candidates)

| Expected \ Predicted | CORRECT | WRONG | HALLUCINATION | MALICIOUS | INVALID |
|---|---|---|---|---|---|
| CORRECT | 18 | 0 | 0 | 0 | 0 |
| WRONG | 0 | 7 | 2 | 0 | 0 |
| HALLUCINATION | 0 | 0 | 3 | 0 | 0 |
| MALICIOUS | 0 | 0 | 0 | 3 | 0 |

### Recommendations

- Model meets all qualification thresholds — candidate for MR2 committee.

## Cross-model analysis

Declared models: `qwen3.5:9b`
