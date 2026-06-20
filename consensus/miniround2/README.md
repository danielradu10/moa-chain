# Mini-Round Two

## Initial Design Notes

Current state

Currently, only the first mini-round is implemented. When the first mini-round is done, we have a canonical set of selected transactions and a canonical map of label frequencies.

Next steps

The flow of the mini-round two should be similar to the next steps:

Each validator runs again a selection algorithm for the consensus group. The algorithm takes into consideration the frequency map from the mini-round one. Each subdomain will have a specific weight, calculated as its frequency / sum (frequencies). This value is multiplied by the score of the validator on that specific domain (which should be known from the validator profile). The result gives the actual number of appearances of the validator in the specific slice (similar with the algorithm for mini-round one).

The leader of the consensus group has the responsibility of collecting all the execution results from the other validators in the consensus, including its own results. This means that here we do not need a first communication with the validators. Each validator after running the selection algorithm can execute the transactions (i.e. calling the agent to answer each prompt) and send each result to the leader, with the corresponding signature. The leader will collect these results and, per each transaction, will have a slice of answers.

Here, the flow is still under debate. There are two options.

The leader just collects the answers. The moment he has 2f+1 valid signed answers, it sends them to the validators. Each validator assures the signatures are correct, which basically means that the answers are actually coming from a valid validator implied in the current consensus round. If each validator is ok with the proposed collection, it commits the block local.

The leader collects the answers but is not searching only for 2f+1 valid signatures. It searches, per transaction, a dominant cluster of answers with at least 2f+1 answers. The clustering algorithm should be deterministic and should give the same result on any node. But in this case, we cannot make an "earlier shot" and send the proposed block whenever 2f+1 signatures are received as we do in the first mini-round. I think we have to wait for all answers... (for a maximum window of time). However, the question comes: what should we do with the transactions which do not have a dominant cluster? We just can't stop the consensus round... we should mark them somehow... Anyway, here the validators will agree also on the dominant cluster, not only on the collection of results.

Why do we need this?

We need a collection of dominant answers per transaction so that the next leader of mini-round three use those answers to create a better answer (i.e. they take the results and go again to the LLM mode and prompt: using these dominant answers received from different agents, create a better one). The validators will have to assure that the proposed leader answer stays in that specific cluster (but this is still under discussion, the focus now is for mini-round two).

## TODO: Phased Implementation Plan

### Phase 1: Answer Evidence Collection

- [ ] Define mini-round two's finalized artifact as signed answer evidence, not yet final semantic answer agreement.
- [ ] Add data structures for expert answers, for example `SignedAnswer`, `TransactionAnswers`, and `AggregatedAnswers`.
- [ ] Define the exact signed payload for each answer. It should commit to epoch, round, mini-round, block hash, transaction hash, answer hash, and expert-selection version.
- [ ] Extend validator profiles with per-subdomain scores.
- [ ] Define deterministic expert selection from mini-round one frequencies and validator domain scores.
- [ ] Use fixed-point integer weights for consensus-visible selection rules; avoid floats in finalized protocol logic.
- [ ] Define canonical ordering and tie-breaking for validators, subdomains, selected experts, and answer payloads.
- [ ] Implement answer execution for selected expert validators.
- [ ] Have each selected validator send signed answers directly to the mini-round two leader.
- [ ] Have the leader collect its own answers and validator answers.
- [ ] Have the leader finalize once it has enough valid signed answer evidence under the chosen quorum rule.
- [ ] Broadcast the aggregated answer evidence to validators.
- [ ] Verify on each validator that signers belong to the expected expert set.
- [ ] Verify all answer signatures against the exact signed payload.
- [ ] Verify every answer references the expected round, block, and transaction.
- [ ] Store the answer evidence certificate or enough proof to audit it later.
- [ ] Add unit tests for expert selection determinism, signature verification, invalid signers, duplicate answers, missing answers, and malformed payloads.
- [ ] Add integration tests for a full mini-round two happy path using deterministic test agents.

### Phase 2: Deterministic Answer Grouping

- [ ] Decide whether answer clustering belongs in mini-round two or should remain an input for mini-round three.
- [ ] Define `AnswerCluster` and explicit per-transaction statuses such as `DominantClusterFound`, `NoDominantCluster`, `InsufficientAnswers`, and `ExecutionFailed`.
- [ ] Define what evidence a cluster must include: answer hashes, signer IDs, cluster ID, and threshold support.
- [ ] Choose a deterministic clustering algorithm that every validator can reproduce from the same evidence.
- [ ] Avoid consensus-critical raw LLM or embedding calls unless model version, deterministic execution, thresholds, and stored outputs are fully specified.
- [ ] Define timeout/window behavior for waiting on all or most expert answers.
- [ ] Define what happens when a transaction has no dominant cluster; do not block the whole consensus round.
- [ ] Make cluster tie-breaking deterministic.
- [ ] Have validators recompute and verify the leader's proposed clusters from the included answer evidence.
- [ ] Store cluster status beside answer evidence so mini-round three can consume it safely.
- [ ] Add unit tests for dominant cluster found, no dominant cluster, insufficient answers, tie-breaking, and deterministic ordering.
- [ ] Add integration tests proving every node finalizes the same answer evidence and the same cluster statuses.
