# MoA Chain experiment summary

This overview is derived from the [cumulative experiment journal](agent-python/testresults/real-agents/README.md), experiment configurations, and recorded run artifacts. The journal and raw artifacts remain authoritative.

## 1. Objective

Characterize the existing full-round heterogeneous-agent protocol under ordinary operation, provider failures, controlled Byzantine MR2 behavior, and an adaptive Byzantine MR3 proposer. The experiments observe safety and liveness without changing quorum or candidate-acceptance rules to obtain a preferred result.

## 2. Question

> Why is a mutex needed when multiple goroutines access shared mutable state? Answer briefly, in at most 5 sentences.

Run 1 used the same question without the final five-sentence instruction; Runs 2–15 used the wording above.

## 3. Base Configuration

| Item | Recorded configuration |
|---|---|
| Validators | `N=10` |
| Committee | Full committee, `G=10` |
| Consensus quorum | `Q=7` |
| MR2 candidate acceptance | 7 `CORRECT` classifications |
| Real providers represented | OpenAI, Anthropic, Gemini, DeepSeek |
| Controlled Byzantine agents in MR2 experiments | Deterministic local mocks with no provider calls |
| Controlled-run timing | MR1 vote-collection deadline `5s`; MR2 grace is scenario-specific below |

Validator substitutions differ by Byzantine scenario and are recorded in the corresponding configs. The MR3 proposer experiment instead used all ten real validators.

## 4. Experiment Scenarios

| Scenario | Perturbation |
|---|---|
| Baseline | Ten real heterogeneous validators; no Byzantine behavior |
| Preprocessing/provider failure | A Gemini label call failed with HTTP 503 |
| 1 Byzantine: WRONG / HALLUCINATION / MALICIOUS | V7 replaced by one deterministic local mock |
| 1 Byzantine WRONG, no grace | First `Q=7` MR2 certificate used the grace period's zero default |
| 1 Byzantine WRONG, grace | Otherwise comparable rerun with `classification_grace_period=10s` |
| 2 Byzantine: WRONG / HALLUCINATION | V7 and v10 replaced by colluding local mocks |
| 2 Byzantine: MALICIOUS | V7 and v8 replaced by colluding local mocks; v10 remained real |
| 3 Byzantine WRONG | V4, v7 and v8 replaced by colluding local mocks |
| Byzantine MR3 proposer | All validators remained real; v10 DeepSeek `deepseek-v4-pro` was forced as MR3 proposer and used an adaptive adversarial synthesis prompt |

## 5. Key Results

| Scenario | MR2 result | Final observation |
|---|---|---|
| Baseline Runs 1–4 | Baseline certificates contained 280/280 `CORRECT` classifications | All four tracked transactions finalized `SYNTHESIZED` |
| Preprocessing/provider failure, Run 5 | Tracked transaction never reached MR2 | Transaction was not selected; run was manually interrupted with no summary or terminal event |
| 1 Byzantine WRONG, no grace, Run 6 | Honest candidates: `6 CORRECT / 1 WRONG`; Byzantine candidate: `1 CORRECT / 6 WRONG` | No candidate reached 7 `CORRECT`; transaction finalized `SKIPPED` with `INSUFFICIENT_CORRECT_ANSWERS` |
| 1 Byzantine WRONG, 10s grace, Run 7 | Honest: `8 CORRECT / 1 WRONG`; Byzantine: `1 CORRECT / 8 WRONG` | Byzantine answer excluded; transaction finalized `SYNTHESIZED` |
| 1 Byzantine HALLUCINATION, Run 8 | Honest: `8 CORRECT / 1 WRONG`; Byzantine: `1 CORRECT / 8 HALLUCINATION` | Byzantine answer excluded; transaction finalized `SYNTHESIZED` |
| 1 Byzantine MALICIOUS, Run 9 | Honest: `8 CORRECT / 1 WRONG`; Byzantine: `1 CORRECT / 5 WRONG / 3 MALICIOUS` | Byzantine answer excluded; transaction finalized `SYNTHESIZED` |
| 2 Byzantine WRONG, corrected collusion, Run 11 | Honest: `7 CORRECT / 2 WRONG`; each Byzantine: `2 CORRECT / 7 WRONG` | Both Byzantine answers excluded; transaction finalized `SYNTHESIZED` |
| 2 Byzantine HALLUCINATION, Run 12 | Honest: `7 CORRECT / 2 WRONG`; each Byzantine: `2 CORRECT / 7 HALLUCINATION` | Both Byzantine answers excluded; transaction finalized `SYNTHESIZED` |
| 2 Byzantine MALICIOUS, Run 13 | Honest: `8 CORRECT / 2 WRONG`; Byzantine candidates: `2/4/4` and `2/5/3` for `CORRECT/WRONG/MALICIOUS` | Both Byzantine answers excluded; transaction finalized `SYNTHESIZED` |
| 3 Byzantine WRONG, Run 14 | Each honest: `7 CORRECT / 3 WRONG`; each Byzantine: `3 CORRECT / 7 WRONG` | Honest answers reached the threshold exactly; wrong answers were excluded; transaction finalized `SYNTHESIZED` |
| Byzantine MR3 proposer, Run 15 | Ten real candidates each received `8 CORRECT / 0 non-CORRECT` from eight complete MR2 votes | V10 generated one subtle false FIFO/fairness claim; eight recorded real evaluators rejected and none approved. |

The first two-Byzantine WRONG run (Run 10) synthesized successfully but did not exhibit intended mutual collusion: each mock approved only its own answer. Run 11 is the corrected colluding comparison above.

Across adversarial candidates, rejection as `non-CORRECT` was more stable than the fine-grained label: honest judges split between `WRONG` and `MALICIOUS` in the malicious scenarios, while still excluding the candidates.

## 6. Provider / Liveness Observations

- Run 5 showed a preprocessing liveness failure after one Gemini HTTP 503 label error: the tracked transaction was never selected and the run required manual interruption.
- Gemini judge failures or incomplete batches affected several runs. Successful runs continued when enough complete MR2 votes remained; provider availability still changed certificate composition.
- With one fast Byzantine judge and no MR2 grace, the first seven-vote certificate contained six honest votes plus the Byzantine vote, limiting honest candidates to six `CORRECT` votes. The 10-second grace rerun collected nine votes and restored liveness for that observed run.
- The grace period is an empirical collection window, not a formal BFT solution. At the `f=3` boundary, every honest candidate required all seven honest `CORRECT` votes; one unavailable, late, or dissenting honest vote could leave it below threshold.
- In Run 15, three Gemini HTTP 504 judge calls made the v7/v8 batches incomplete, but eight other complete votes allowed MR2 to finish. MR3 then recorded `0` approvals and `8` rejections; the missing v6 evaluation could not make the six-approval quorum reachable.

## 7. Main Conclusions

- The completed baseline runs show that the real heterogeneous committee can carry the tracked transaction through MR1, MR2, and MR3 under the recorded conditions.
- Controlled Byzantine MR2 candidates remained outside `correct_answers` in the successful 1-, 2-, and 3-Byzantine runs. This is observed safety evidence, not a general proof.
- Liveness depends on certificate composition and complete honest votes. The no-grace/grace comparison demonstrates that additional collection time helped one observed run, while the `f=3` result shows that the fixed seven-`CORRECT` threshold leaves no spare honest vote.
- Provider/API availability is a separate liveness concern from Byzantine semantic safety: incomplete real-model batches can remove otherwise honest votes.
- Run 14 is one successful full-certificate sample at `f=3`; it does not establish unconditional liveness at that boundary.
- The adaptive MR3 proposal was conclusively rejected by every recorded evaluator, but the manually interrupted run has no terminal MR3 or transaction status and must not be reported as a completed lifecycle.
