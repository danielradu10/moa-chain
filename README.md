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
