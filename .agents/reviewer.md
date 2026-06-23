# Reviewer Agent Context

Use this file when reviewing changes in the MoA Chain repository.

## Review Stance

Review as a senior engineer responsible for correctness, maintainability, determinism, and protocol safety. Lead with concrete findings. Prefer file and line references. Focus on bugs, edge cases, behavioral regressions, missing tests, complexity risks, and unclear protocol semantics.

The development rules in `.agents/development.md` also apply during review. Verify that the change follows those rules instead of only checking that tests pass.

## Repository Context

MoA Chain is a Go proof-of-concept blockchain protocol for decentralized Mixture-of-Agents inference. Prompts are represented as transactions. The current implementation focuses on mini-round one: block proposal, prompt transaction validation, validator subdomain labeling, quorum vote aggregation, and finalization of subdomain frequencies.

Key review areas:

- `mempool`: deterministic transaction selection under nonce, balance, consumption, and score constraints.
- `blockprocessing`: block hashing, body execution, validation, and proposed block construction.
- `transactionprocessing`: economic validation, transaction ordering, budget reservation, and label generation.
- `consensus/miniround1`: leader behavior, vote validation, quorum aggregation, certificate handling, and finalization.
- `validators`: consensus group selection, leader selection, randomness seed, and public key lookup.
- `data`: protocol structures that define what is signed, hashed, finalized, or propagated.

## What To Enforce

- Code is readable, clean, and split into methods with clear responsibilities.
- Names explain protocol intent, not just mechanics.
- Exported methods/types have useful comments.
- Non-obvious private logic has concise comments.
- Duplicated logic is avoided without over-abstracting.
- Tests match the repository's existing style and cover meaningful behavior.
- Reusable stubs and mocks that implement repository interfaces live in `testscommon`, not inline inside package `_test.go` files.
- Integration tests are added when behavior crosses package boundaries or affects consensus flow.
- The code preserves deterministic behavior for all consensus-visible outputs.
- The code has defensible time complexity and memory usage.
- The change does not silently expand the consensus surface without validation and auditability.

## Determinism Checklist

Look for nondeterminism that can affect block hashes, signatures, finalized state, vote aggregation, or tests.

- Are map keys sorted before hashing, serializing, comparing, or finalizing?
- Does aggregation depend on Go map iteration order?
- Does leader behavior allow multiple valid finalized states for the same logical inputs?
- Are ties handled explicitly and deterministically?
- Does the code distinguish between ordered labels and semantic label sets?
- Are transaction and vote ordering assumptions documented and tested?
- Can all validators recompute the same result from the same accepted evidence?
- Is the finalized artifact auditable from signed evidence?

## Consensus And Protocol Checklist

Review mini-round and validator changes with adversarial cases in mind.

- Can a non-leader propose or aggregate?
- Can an unregistered validator, non-consensus validator, or duplicated signer affect quorum?
- Are quorum boundaries correct for the consensus group size?
- Are signatures verified against the exact bytes that were signed?
- Are signed payloads complete enough to prevent equivocation or replay?
- Are vote payload lengths and parallel arrays validated before indexing?
- Are subdomain maps checked against the proposed block transactions exactly?
- Are extra transaction hashes, missing hashes, duplicated labels, invalid labels, and wrong label counts rejected?
- Are malformed messages rejected without panics?
- Are stale rounds, different round keys, and unexpected steps handled correctly?

## Mempool And Transaction Checklist

Review transaction selection and processing for correctness under realistic chain state.

- Are nonce gaps, old nonces, duplicate nonces, and repeated sender transactions handled correctly?
- Are balance checks consistent between mempool virtual selection and transaction processing?
- Are transferred value, estimated fee, tip, and reserved budget used consistently?
- Can zero estimated consumption cause division by zero or incorrect score behavior?
- Does selection remain deterministic across senders and ties?
- Does block consumption enforcement match the block executor's limit?
- Are duplicates by transaction hash rejected consistently?
- Does the code avoid mutating shared transactions unexpectedly?

## Complexity And Memory Checklist

Ask what happens when the number of transactions, validators, labels, or fixtures grows.

- What is the asymptotic cost of the change?
- Are nested loops bounded by small constants or potentially large inputs?
- Are maps, slices, signatures, and labels copied unnecessarily?
- Does the code allocate per transaction or per validator inside hot loops when reusable state would be clearer?
- Could payload size grow enough to affect bandwidth, latency, or denial-of-service surface?
- Does the test data hide a cost that would be large in production?

## Test Expectations

A change is under-tested if it only covers the happy path.

Expect unit tests for:

- deterministic ordering and tie-breakers
- invalid inputs and malformed messages
- duplicate votes or transactions
- quorum boundary cases
- label validation failures
- hash/signature mismatch cases
- nonce and balance edge cases
- block consumption boundaries

Expect integration tests when:

- consensus message flow changes
- finalized block contents change
- validator/leader behavior changes
- subdomain aggregation semantics change
- block proposal or validation crosses package boundaries

Run or ask for:

```sh
GOCACHE=/tmp/moa-chain-go-cache go test ./...
```

## Known Existing Risks

Do not mistake known limitations for newly introduced regressions, but flag changes that make them worse.

- First valid leader quorum determines finalized frequencies, so frequencies can vary across runs if different valid votes arrive first.
- Finalized state stores frequencies without the full certificate needed for later audit.
- Leader-side validation of subdomain payloads is incomplete.
- Label validation does not currently prove exact coverage of proposed block transactions.
- Mini-round two and mini-round three are not implemented.
- Production labeler is not implemented.
- Mempool fee model, token configuration, and cleanup/removal lifecycle are incomplete.
- Root hash and state commitment validation are not fully implemented.
