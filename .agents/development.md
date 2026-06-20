# Development Agent Context

Use this file when implementing changes in the MoA Chain repository.

## Repository Context

MoA Chain is a Go proof-of-concept blockchain protocol for decentralized Mixture-of-Agents inference. User prompts are modeled as transactions. The current implementation focuses on mini-round one: selecting a consensus group, proposing a block, validating prompt transactions, labeling them with coding subdomains, collecting validator votes, and finalizing aggregated subdomain frequencies.

Important packages:

- `data`: shared block, transaction, round, consensus message, and subdomain types.
- `mempool`: deterministic prompt transaction selection using sender state, nonce ordering, estimated consumption, and score.
- `blockprocessing`: block proposal, body execution, validation, hashing, and finalization.
- `transactionprocessing`: transaction economic validation, reservation, ordering validation, and labeler integration.
- `consensus/miniround1`: mini-round one flow, vote verification, aggregation, and finalization.
- `validators`: deterministic consensus group and leader selection.
- `integrationtests`: end-to-end mini-round one tests and label fixtures.

The attached first paper is titled "MoA Chain: An Architecture for a Blockchain Protocol Supporting a Decentralised Mixture of Agents". It describes prompts as transactions and a multi-step consensus flow: prompt/subdomain consensus, expert agent routing/execution, answer comparison, canonical response validation, and reward/penalty accounting. The repository currently implements and tests the first mini-round.

## Engineering Rules

Write code that can be understood by a new engineer reading the repository for the first time.

- Prefer clean, direct Go code over clever abstractions.
- Split complex logic into small methods with one clear responsibility.
- Use meaningful names for methods, variables, types, and tests.
- Avoid duplicated code. Extract helpers only when they remove real repetition or clarify behavior.
- Keep changes scoped to the package and behavior requested.
- Follow the current repository patterns before introducing new ones.
- Comment exported methods, types, and interfaces.
- Add short comments for non-obvious private methods or complex protocol rules.
- Do not add comments that merely repeat the code.
- Preserve deterministic behavior wherever consensus, hashing, ordering, selection, or aggregation is involved.
- Think explicitly about time complexity, memory usage, and data-structure behavior before implementing.
- Prefer structured validation over ad hoc checks.
- Treat protocol edge cases as first-class behavior, not incidental details.

## Determinism Rules

This repository contains consensus and hashing code. Any nondeterminism can become a protocol bug.

- Sort map keys before hashing, serializing, aggregating, or comparing when order matters.
- Do not rely on Go map iteration order for finalized state, signatures, block hashes, label frequencies, or test expectations.
- Preserve transaction ordering rules already used by `mempool`, `transactionprocessing`, and `blockprocessing`.
- When adding aggregation logic, define exactly what evidence is accepted and how ties are handled.
- When changing label/subdomain behavior, define whether labels are ordered lists or semantic sets.
- Ensure every validator that receives the same accepted evidence computes the same result.
- If leader-selected evidence affects state, make the selected evidence auditable or derive state with a deterministic threshold rule.

## Complexity Rules

Before writing code, reason about the expected input sizes and costs.

- Consider transaction count per block, validators in a consensus group, labels per transaction, and map sizes.
- Prefer O(n log n) sorting only where deterministic order is needed.
- Avoid repeated full scans in nested loops when a map/set can express the rule clearly.
- Avoid unnecessary copying of transactions, labels, signatures, and vote payloads.
- Be careful with memory growth in aggregation and integration-test fixtures.
- For consensus paths, consider bandwidth and payload-size impact, not only local CPU cost.

## Testing Rules

Add tests when behavior changes. Match the current repository style.

- Add focused unit tests near the package under change.
- Add integration tests only when behavior crosses packages or changes the mini-round flow.
- Use `testify/require`, table tests, and local helper constructors consistently with existing tests.
- Test edge cases, not just the happy path.
- For deterministic logic, test ordering and stable output explicitly.
- For consensus logic, test invalid signers, wrong leaders, duplicate votes, missing evidence, malformed labels, quorum boundaries, and different vote arrival orders when relevant.
- For mempool logic, test nonce gaps, duplicate hashes, balance limits, block consumption limits, sender ordering, score ties, and deterministic tie-breakers.
- Run `GOCACHE=/tmp/moa-chain-go-cache go test ./...` before finishing when possible.

## Current Caveats To Keep In Mind

- Mini-round one finalizes frequencies from the first valid quorum assembled by the leader. This is deterministic for a chosen certificate, but not necessarily run-to-run deterministic.
- Finalized `BlockOnChain` stores `SubdomainsFrequencies`, but does not currently persist the full aggregated certificate for later audit.
- Leader-side vote validation verifies block signatures, but stronger subdomain payload validation is still marked as future work.
- `LabelsValidator.ValidateLabels` checks count, membership, and duplicates, but currently does not ensure the subdomain map exactly matches the proposed block transactions.
- Mini-round two and mini-round three are not implemented.
- Production labeler implementation is a stub.
- Mempool token configuration and fee model are incomplete.
- Root hash/state commitment logic is still TODO-level.
