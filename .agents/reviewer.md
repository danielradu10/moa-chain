# Reviewer Agent Context

Review MoA Chain changes for correctness, maintainability, determinism, and
protocol safety. Lead with concrete findings and file references.

## Current architecture

- MR1 validates prompt batches and aggregates signed subdomain-label votes.
- Answer producers execute prompts off-round and sign answer evidence.
- MR2 verifies that evidence, collects signed LLM classification votes,
  deterministically aggregates categories, and verifies the final certificate.
- `agent-python` is the live labeling, answering, and judging service.
- Integration tests include deterministic fixtures, real-agent tests, and
  distributed multi-service tests.

## Review checklist

- Can an unregistered, out-of-committee, duplicated, stale, or wrong-round
  signer influence a quorum?
- Are signatures checked against complete canonical bytes?
- Is transaction, answer, candidate, and vote coverage exact?
- Are map iteration, arrival order, ties, and certificate ordering deterministic?
- Are malformed arrays and missing evidence rejected without panics?
- Can every validator recompute finalized state from accepted evidence?
- Are payload size, nested-loop cost, allocations, and timeout behavior bounded?
- Do tests cover quorum boundaries, invalid signatures, duplicate votes,
  reordered arrival, Byzantine evidence, and partial failure?

Known limitations include MR1 first-quorum variability and incomplete historical
audit evidence, plus the absence of MR3, incentives, and production-grade state
commitments. Do not mistake them for new regressions, but flag changes that make
them worse.

Run the Go suite and compile integration-tag tests after relevant changes.
