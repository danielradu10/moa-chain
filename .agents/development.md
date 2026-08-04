# Development Agent Context

Use this file when implementing changes in MoA Chain.

## Repository context

MoA Chain is a Go proof-of-concept blockchain protocol for decentralized
Mixture-of-Agents inference. The repository implements:

- mini-round one consensus over prompt batches and subdomain-label evidence;
- off-round answer generation with signed producer evidence;
- mini-round two LLM judging, signed classification votes, deterministic
  aggregation, certificate verification, and finalization;
- a FastAPI agent service in `agent-python` for labeling, answering, and judging;
- deterministic, real-agent, and distributed integration-test architectures.

Important packages are `data`, `mempool`, `blockprocessing`,
`transactionprocessing`, `consensus/miniround1`, `consensus/miniround2`,
`validators`, `agent/httpclient`, and `integrationtests`.

## Engineering rules

- Prefer direct, readable Go and small methods with one responsibility.
- Preserve existing package patterns and keep changes scoped.
- Comment exported APIs and non-obvious protocol rules.
- Treat consensus-visible ordering, hashing, signatures, evidence, and
  aggregation as deterministic protocol behavior.
- Sort map-derived data before hashing or serializing when order matters.
- Define quorum boundaries, tie behavior, evidence coverage, and malformed-input
  handling explicitly.
- Consider transaction, validator, candidate, signature, and payload growth.
- Put reusable test doubles in `testscommon`.
- Add focused unit tests and integration tests for cross-package protocol flow.

## Current caveats

- MR1 finalizes the first valid quorum certificate and does not persist its full
  label evidence, so results are auditable only within the active flow.
- MR1 label frequencies can vary across runs when valid votes differ.
- MR2 depends on nondeterministic LLM judgments but treats them as signed votes;
  only deterministic vote aggregation becomes finalized state.
- Mini-round three, incentives, complete fee accounting, and production state
  commitments are not implemented.

Run `GOCACHE=/tmp/moa-chain-go-cache go test ./...` before finishing when
possible. Compile integration-tag tests after changing shared fixtures or
consensus interfaces.
