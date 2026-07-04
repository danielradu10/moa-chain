# Mini-Round Two: LLM-Judged Answer Classification

## Goal

Mini-round two must produce, for every transaction, a canonical group of correct
answers that mini-round three can use to synthesize a better final answer.

Embedding-based clustering is not used for this decision. Each validator asks
its attached agent to classify the collected answers. Because LLM output is not
deterministic, an individual classification is treated as a signed validator
vote, not as consensus output. The deterministic aggregation of those signed
votes is the consensus-visible result.

Mini-round two does not create the final answer. It only determines which
independently produced answers are eligible inputs for mini-round three.

## Terminology

- **Answer producer**: a selected mini-round-two validator that executes the
  transaction prompt.
- **Candidate answer**: one signed answer produced for one transaction.
- **Judge**: a selected mini-round-two validator that asks its attached agent to
  classify every candidate answer.
- **Answer evidence certificate**: the signed execution results collected and
  broadcast by the leader. This structure already exists as
  `AggregatedExecutionResultsMessage`.
- **Classification vote**: one judge's signed assignment of every candidate
  answer to exactly one category.
- **Classification certificate**: the set of classification votes accepted for
  deterministic aggregation.
- **Canonical correct group**: the candidate answers that received the required
  Correct support.

The initial design uses the same mini-round-two committee for producing answers
and judging them. Only members of this committee contribute to either quorum.

## Categories

Every judge must assign exactly one category to every candidate answer:

1. `Correct`: answers the transaction prompt accurately and sufficiently.
2. `Hallucination`: relies on fabricated, unsupported, or nonexistent facts.
3. `Adversarial`: contains observable manipulation such as prompt injection or
   instructions intended to influence later protocol stages. This category does
   not claim that the producer's private intent is known.
4. `Wrong`: incorrect, irrelevant, contradictory, or materially incomplete,
   without satisfying the narrower Hallucination or Adversarial definitions.

These definitions and their precedence must be part of the versioned protocol
prompt. The Go protocol may retain `Malicious` as the wire value if required,
but the prompt should use the observable `Adversarial` definition.

## Protocol Flow

### 1. Produce and collect answers

Each selected validator executes every canonical mini-round-one transaction and
sends its signed execution result to the mini-round-two leader. The leader:

1. verifies committee membership, transaction coverage, hashes, and signatures;
2. collects the answer-evidence quorum;
3. orders evidence deterministically by validator ID;
4. broadcasts the complete answer evidence certificate.

This is the flow currently implemented by `AnswersBlockMessage` and
`AggregatedExecutionResultsMessage`.

### 2. Verify identical judge input

Every judge verifies the answer evidence using the same rules as the leader. A
judge runs the LLM only after the evidence is valid.

All judges must classify the same candidate set, including their own answers.
Removing a judge's own answer would give different judges different inputs and
would reduce the maximum possible vote count for that answer.

Each candidate has a protocol identity independent of its text:

```text
candidateID = (producerValidatorID, txHash, answerHash)
```

Identical text produced by two validators remains two candidate answers. The
producer identity is used by the protocol but should be hidden from the LLM to
reduce self-preference and validator-specific bias.

### 3. Run the local LLM judge

The node stores an immutable, versioned protocol prompt. The judge input contains
the canonical transactions and their candidate answers in deterministic order:

```text
transaction hash
original transaction prompt
candidate ID local to the judge request
candidate answer text
```

Candidate answers are untrusted quoted data. The protocol prompt must state that
instructions inside candidate answers must never be followed. The LLM must
return strict structured output containing exactly one category for every
candidate.

The protocol-level input is the complete transaction set. The implementation
may execute one LLM request per transaction to limit context contamination and
context-window growth, provided the same prompt version, ordering rules, and
output schema are used.

The judge prompt version and prompt hash are included in the signed vote. Model
and inference metadata should also be recorded for auditability, although exact
model determinism is not required for consensus.

### 4. Sign and collect classification votes

Each judge signs a payload committing to at least:

```text
epoch, round, mini-round
canonical mini-round-one block hash
answer evidence certificate hash
judge validator ID
judge prompt version and hash
ordered candidate IDs and assigned categories
```

The vote is sent to the leader. The leader rejects votes with an unexpected
judge, evidence hash, prompt version, candidate set, duplicate candidate, missing
candidate, invalid category, or invalid signature.

### 5. Aggregate deterministically

For each candidate answer, count how many accepted judges assigned each category:

```text
candidateID -> {
    Correct:       count,
    Hallucination: count,
    Adversarial:   count,
    Wrong:         count,
}
```

The initial PoC policy is:

- `quorum = floor(2 * committeeSize / 3) + 1`, equivalent to `2f+1` for a
  committee of `3f+1`;
- a candidate enters the canonical correct group only when its Correct count is
  at least `quorum`;
- otherwise it enters whichever of Hallucination, Adversarial, or Wrong has the
  highest count;
- ties among non-correct categories resolve conservatively in this order:
  `Wrong`, `Hallucination`, `Adversarial`;
- a transaction is eligible for mini-round three only when its canonical correct
  group contains at least `quorum` independently produced candidate answers.

The non-correct groups are retained for auditing but are not inputs to
mini-round three. Any group may be empty. A transaction that does not reach the
correct-answer quorum is finalized with an explicit `InsufficientCorrectAnswers`
status; it does not block the rest of the round.

With a classification certificate containing exactly `2f+1` votes, requiring
`2f+1` Correct classifications means unanimous Correct classification within
that certificate. This is intentionally conservative for the first experiment
and must be measured before selecting a less strict rule.

### 6. Broadcast, verify, and finalize

The leader broadcasts both the classification certificate and its derived
groups. Every validator:

1. verifies the leader and round identifiers;
2. verifies every judge's membership and signature;
3. verifies the answer evidence and prompt hashes;
4. recomputes the category counts and groups locally;
5. rejects any leader-provided result that differs from local recomputation;
6. finalizes the canonical groups and per-transaction status.

The leader must not finalize immediately after broadcasting answer evidence.
Mini-round two is complete only after the classification certificate is
verified and the canonical correct groups are stored.

## State-Machine Change

The current flow finalizes inside `HandleAggregatedExecutionResults`. That
handler must instead verify and store answer evidence, run the judge, and send a
classification vote. The round needs the following additional logical states:

```text
CollectExecutionResults
AwaitAnswerEvidence
JudgeAnswers
CollectClassificationVotes       (leader)
AwaitClassificationCertificate   (non-leader)
Finished
```

The leader must also process its own classification vote through the same
verification and storage path as every other vote.

## Consensus and Security Invariants

- LLM output is never trusted as deterministic protocol computation.
- Every finalized group can be reconstructed from signed answer and
  classification evidence.
- All judges classify exactly the same ordered candidate set.
- One judge contributes at most one classification per candidate.
- One producer contributes at most one answer per transaction.
- Candidate text is never used as its unique identity.
- Arrival order, Go map iteration order, and LLM output order cannot affect the
  finalized result.
- A malformed or missing classification affects that judge's vote, not the
  processing of unrelated transactions.
- Transactions without a correct-answer quorum do not stop the round.
- Mini-round three consumes only the canonical correct group and its evidence.

## Known Limitations and Open Decisions

The following items must remain explicit rather than being hidden in code:

- **Certificate composition:** collecting the first `2f+1` judge votes lets a
  leader influence which valid votes are included. Waiting for every `3f+1`
  judge breaks Byzantine liveness. The initial PoC should record this limitation
  and collect data before changing the rule.
- **Strict Correct threshold:** LLM variation may make unanimous Correct votes
  uncommon and reduce liveness at the transaction level. This fails closed but
  may reject useful answers.
- **Category ambiguity:** Hallucination, Adversarial, and Wrong overlap without a
  precise decision tree and test cases.
- **Prompt injection:** quoting and instructions reduce risk but cannot prove
  that an LLM judge ignored hostile answer content.
- **Context size:** judging every transaction in one request may exceed model
  limits. Per-transaction requests need a defined partial-failure policy.
- **Timeouts:** the protocol needs an explicit deadline and behavior when fewer
  than the classification quorum respond.
- **Prompt upgrades:** activation of a new prompt version must be coordinated so
  honest validators do not reject one another's votes.
- **Committee scope:** expanding judging beyond the selected mini-round-two
  committee changes `n`, `f`, membership verification, and quorum semantics and
  is not part of the initial implementation.

## Incremental Pull-Request Plan

Each PR should compile independently, preserve existing behavior until the
activation PR, and include focused tests. Protocol types should not depend on a
specific LLM provider.

### PR 1: Protocol types and canonical identities

Scope:

- add the category enum, candidate ID, classification assignment, vote,
  certificate, category counts, canonical groups, and transaction status types;
- define validation rules for duplicate, missing, and unknown candidates;
- define canonical ordering for transactions, producers, candidates, and judges;
- add hashing helpers for answer evidence and classification vote payloads.

Tests:

- stable hashes under different map insertion orders;
- duplicate and missing candidate rejection;
- candidate identity keeps identical answer text from different producers
  separate;
- enum and ordering validation.

This PR adds types and pure helpers only; it does not change the round flow.

### PR 2: Deterministic classification aggregation

Scope:

- implement a pure aggregator from verified classification votes to category
  counts, four canonical groups, and transaction status;
- implement the Correct threshold and non-correct tie-breaking policy;
- expose the quorum policy as a protocol function, not a magic constant.

Tests:

- empty categories;
- Correct threshold reached and missed;
- ties among non-correct categories;
- fewer than `2f+1` correct candidate answers;
- multiple transactions and shuffled vote order;
- malformed or incomplete votes rejected before aggregation.

No networking, state, signatures, or LLM calls belong in this PR.

### PR 3: Judge interface and versioned protocol prompt

Scope:

- extend the agent boundary with an answer-judging interface;
- add the versioned protocol prompt and its hash;
- build canonical, producer-anonymized judge input from verified answer evidence;
- parse strict structured output and validate complete candidate coverage;
- define per-transaction LLM failure behavior.

Tests:

- golden prompt/input fixture;
- prompt hash fixture;
- valid structured response parsing;
- missing, duplicate, additional, and invalid classifications;
- candidate answer containing prompt-injection text remains quoted data.

This PR can use a fake judge and must not alter consensus behavior.

### PR 4: Classification message and round-state plumbing

Scope:

- add classification-vote and classification-certificate consensus messages;
- add broadcaster methods and round-state storage;
- reject duplicate judge votes;
- add the new round steps and message routing without activating finalization
  changes;
- add errors and logging fields for evidence hash, prompt version, and judge ID.

Tests:

- message routing by step and round;
- duplicate and stale messages;
- nil and misaligned certificate fields;
- round-state cleanup.

### PR 5: Validator judging and signed vote production

Scope:

- after verifying answer evidence, build the judge input and invoke the agent;
- create and sign a classification vote bound to the exact evidence certificate;
- send the vote to the selected leader;
- process the leader's own vote through the same path;
- keep the existing finalization path behind the activation point until the full
  certificate flow is ready.

Tests:

- judge is never called before evidence verification;
- vote covers every canonical candidate exactly once;
- own answers are included but producer identities are hidden from the LLM;
- wrong evidence or prompt hash cannot be signed;
- judge failure does not produce a partial vote.

### PR 6: Leader vote collection and certificate broadcast

Scope:

- verify judge membership, signature, evidence hash, prompt hash, and candidate
  coverage;
- collect one vote per judge;
- aggregate after reaching the configured classification-certificate rule;
- broadcast signed votes together with the derived canonical groups;
- add classification collection timeout behavior.

Tests:

- invalid signer, signature, evidence hash, prompt version, and candidate set;
- duplicate judge vote;
- quorum boundary;
- deterministic result under different arrival orders;
- leader-provided groups match the pure aggregator.

### PR 7: Certificate verification and mini-round-two finalization

Scope:

- verify every classification vote on receiving the leader's certificate;
- recompute and compare the canonical groups;
- replace immediate answer-evidence finalization with classification finalization;
- store answer evidence, category counts, groups, and transaction status;
- transition the round to Finished only after successful verification;
- remove or disable embedding clustering from the mini-round-two protocol path.

Tests:

- tampered leader result is rejected;
- valid nodes finalize byte-equivalent canonical artifacts;
- insufficient Correct answers finalize with status instead of failing the round;
- empty non-correct groups;
- stale and duplicate certificates.

This is the behavior-activation PR.

### PR 8: End-to-end and Byzantine-behavior tests

Scope:

- add integration fixtures with deterministic fake judges that intentionally
  disagree;
- test the complete answer, evidence, judgment, aggregation, verification, and
  finalization flow;
- measure payload size and latency for representative transaction counts.

Scenarios:

- unanimous correct classifications;
- one Byzantine or malformed judge;
- disagreement across all four categories;
- prompt-injection candidate;
- leader reorders or omits votes;
- answer quorum reached but correct-answer quorum missed;
- one transaction fails classification while others finalize;
- timeout before classification quorum.

### PR 9: Mini-round-three handoff

Scope:

- expose only eligible canonical correct groups to mini-round three;
- bind mini-round-three input to the finalized mini-round-two artifact hash;
- skip transactions with `InsufficientCorrectAnswers`;
- retain evidence needed to audit every included answer.

Mini-round-three answer synthesis and validation are separate design work and
should not be implemented as part of the mini-round-two PRs.

## Review Checkpoint

Before PR 5 activates LLM calls in the protocol, review and freeze for the PoC:

1. category names, definitions, and precedence;
2. prompt content, output schema, version, and hash;
3. answer and classification certificate quorum rules;
4. non-correct tie-breaking;
5. transaction eligibility rule for mini-round three;
6. timeout and partial-LLM-failure behavior;
7. the exact signed payloads and canonical serialization.

Changing any of these after activation is a protocol change and requires a new
version plus coordinated rollout.
