# MoA Chain

A proof-of-concept blockchain protocol for distributed LLM inference and semantic consensus. 

MoA Chain explores how a network of validators can collaboratively execute, compare, and validate LLM-generated responses in order to produce a canonical, improved answer for a given prompt.

## Determinism note: subdomain frequencies

The current mini-round one flow finalizes subdomain frequencies from the first valid quorum certificate assembled by the leader. This gives node-to-node determinism once the certificate is chosen: every validator that accepts the same aggregated votes computes the same `SubdomainsFrequencies`.

However, the result is not necessarily run-to-run deterministic. The leader finalizes as soon as quorum is reached, so the aggregated certificate can contain whichever valid validator votes arrived first. If validators produce different labels for the same transactions, different valid quorums can produce different frequency maps.

Example:

- Validator A labels a transaction with `security`, `databases`, and other labels.
- Validator B labels the same transaction with `security`, `cloud_engineering`, and other labels.
- In one run, A's vote reaches the leader before B's vote and is included in the first quorum.
- In another run, B's vote reaches the leader before A's vote and is included instead.

Both runs can produce valid quorum certificates, and all nodes in each run will agree on the certificate they receive. But the finalized subdomain frequencies can differ between runs because the included validator set differs.

This is acceptable only if the protocol explicitly defines the first valid quorum certificate as the source of truth for subdomain frequencies. If the intended state should represent a timing-independent consensus over labels, the protocol should derive final labels or frequencies from a deterministic rule, such as thresholding labels per transaction by quorum support, waiting for a defined validator set or timeout policy, or storing the exact certificate signer set as part of the finalized artifact.

## Design risks: validators sending subdomains in votes

The current mini-round one design makes validators send their generated subdomains as part of their block votes. Validators sign the block hash and also sign their own `txHash -> labels` subdomain map. The finalized chain artifact then stores aggregated subdomain frequencies derived from the vote payloads.

This design can work, but it expands the consensus surface beyond a simple block-hash vote. The protocol must define very clearly what the final subdomain state means and how it can be verified.

Important risks and failure points:

- The leader controls which valid label votes enter final state. If several valid quorums exist, the leader can finalize using the first quorum or choose a quorum whose labels produce preferred frequencies.
- A malicious leader can censor inconvenient label votes while still finalizing with quorum.
- Raw frequencies are not a canonical final label decision. It is unclear whether they represent evidence, a ranking signal, or finalized semantic consensus.
- Vote payloads become larger because each vote includes labels for every transaction. This increases bandwidth, latency, and denial-of-service surface.
- Verification becomes more expensive because validators must validate label maps, hash subdomains, and verify extra signatures.
- A validator can submit any six unique labels from the allowed set. Current validation checks shape and membership, but not semantic correctness.
- The labels validator must ensure the subdomain map exactly matches the proposed block: every block transaction must have labels, no transaction may be missing, and no extra transaction hash may be included.
- Label ordering matters if the hash uses the label slice order. If labels are intended to be semantic sets, order-sensitive hashing can make equivalent labels produce different signatures.
- If final state stores only `SubdomainsFrequencies`, later nodes cannot audit which validators produced those frequencies unless the full aggregated certificate or equivalent proof is also stored.
- Replaying or verifying historical chain state requires enough data to prove that the frequencies came from a valid quorum. Frequencies alone are not sufficient.
- If labels affect economics, rewards, routing, reputation, or task categorization, validators may have incentives to submit strategically biased labels.
- Semantic disagreement is aggregated, but the protocol does not yet define when a label has enough support to become accepted.

Recommended design directions:

- If the final state is meant to be "the first valid quorum certificate", store the certificate or enough information to audit it: signer set, signatures, and signed subdomain maps.
- If the final state is meant to be canonical labels per transaction, derive them deterministically. For example, accept a label only if it appears in at least a quorum of votes for that transaction, then store accepted labels in canonical sorted order.
- If raw frequencies remain part of the state, explicitly define the source of those frequencies and make it auditable.

The strongest safety requirement is that finalized semantic state should either be directly verifiable from an included certificate or derived by a deterministic threshold rule. Raw leader-selected quorum frequencies should not be treated as standalone chain state unless the quorum evidence is also preserved.
