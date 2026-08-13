# Mini-round-three integration scenarios

This directory contains deterministic fixtures for the complete MR1→MR2→MR3
flow. Each scenario supplies controlled labels, answers (MR2), synthesis outputs
(MR3), evaluation behaviors, network parameters, and expected protocol outcomes.
The suite uses fake synthesis and evaluation agents so consensus behavior can
be reproduced without an LLM service.

---

## Protocol recap (what MR3 adds)

MR3 receives the finalized MR2 block and:

1. **Committee selection** — same algorithm as MR2 but keyed on the MR3 round
   key, so the two committees may differ.
2. **Leader synthesizes** — calls `SynthesizeBatch` over all eligible
   transactions (those with `READY_FOR_MINI_ROUND_THREE` status in MR2),
   builds a `ProposedSynthesisMessage`, signs and broadcasts it.
   The leader stores its own proposal but does **not** vote.
3. **Validators evaluate** — non-leader committee members call
   `EvaluateSynthesisBatch`; if all answers are approved they send a signed
   `SynthesisVote` to the leader. Validators that reject or error send no vote
   (silent abstention).
4. **Leader aggregates** — at `floor(2n/3)` valid votes the leader builds
   `AggregatedSynthesisVotes`, finalizes its own block, and broadcasts the cert.
5. **Non-leaders finalize** — receive the cert, verify every vote, finalize the
   block with `FinalAnswers` (`SYNTHESIZED` or `SKIPPED`).

**Quorum formula for MR3:** `(2 × committeeSize) / 3` — with committeeSize=10
this is **6**, one less than the MR1/MR2 formula (`floor(2n/3) + 1 = 7`).

---

## Network parameters (all scenarios)

| Parameter | Value |
|---|---|
| Registered nodes | 20 |
| Committee size | 10 |
| MR3 quorum (synthesis votes) | 6 |

---

## Fixture format

Each scenario lives in its own directory matching the `name` field and contains
a `scenario.json`. The schema extends the MR2 fixture with three new top-level
sections. Unknown fields fail validation.

```jsonc
{
  // ── shared with MR2 ──────────────────────────────────────────────
  "name": "...",
  "description": "...",
  "network": { "registeredNodes": 20, "committeeSize": 10, "quorum": 7 },
  "transactions": [ /* 3 entries, same schema as MR2 */ ],
  "executors": [ /* same schema as MR2 */ ],
  "judges":    [ /* same schema as MR2 */ ],

  // ── MR3-specific ────────────────────────────────────────────────
  "synthesizer": {
    // Leader's SynthesizeBatch output per txHash.
    // Only eligible txs (READY_FOR_MINI_ROUND_THREE) need an entry.
    "answers": {
      "<txHash>": "<synthesized answer text>",
      ...
    }
  },
  "evaluators": [
    {
      // role must include "default" as the catch-all.
      "role": "default" | "member-N",
      // approve_all   → EvaluateSynthesisBatch returns Approved:true for every request
      // reject_all    → returns Approved:false for every request (silent abstention)
      // execution_error → returns error for the errorTxHash entry (abstains entirely)
      "mode": "approve_all" | "reject_all" | "execution_error",
      "errorTxHash": "<txHash, only for execution_error mode>"
    },
    ...
  ],

  "delivery": {
    // ── MR2 delivery (same keys as MR2 fixture) ──
    "producerOrder": [ "leader", "member-1", ... ],
    "judgeOrder":    [ "leader", "member-1", ... ],
    "answerEvidenceFaults":            [],
    "classificationVoteFaults":        [],
    "classificationCertificateFaults": [],

    // ── MR3 delivery (new keys) ──
    "synthesizerVoteOrder": [ "leader", "member-1", ... ],
    "synthesisVoteFaults": [
      // type: "drop" | "invalid_signature" | "wrong_synthesis_hash"
      { "senderRole": "member-N", "type": "..." }
    ],
    "synthesisProposalFaults": [
      // Applied to the leader's broadcast at the network boundary.
      // type: "mutate_proposal_hash"
      { "type": "..." }
    ]
  },

  "expected": {
    // ── MR2 expected (same keys) ──
    "roundFinalized": true,
    "finalizedNodes": 20,
    "answerEvidenceProducers": [ ... ],
    "classificationVoters": [],
    "transactions": [ /* MR2-level per-tx outcomes */ ],

    // ── MR3 expected (new keys) ──
    "mr3RoundFinalized": true | false,
    "mr3FinalizedNodes": 20 | <partial>,
    "mr3ErrorContains": "",         // non-empty when mr3RoundFinalized is false
    "synthesisVoters": [ ... ],     // exact voters when mr3RoundFinalized is true
    "finalAnswers": [
      {
        "txHash": "...",
        "status": "SYNTHESIZED" | "SKIPPED",
        "answer": "..."             // empty when status is SKIPPED
      }
    ]
  }
}
```

---

## Scenarios

### 1 — `unanimous_synthesis_approval`

**What it tests:** baseline happy path: all three transactions reach MR3, the
leader synthesizes all of them, every validator approves, the block finalizes
with all answers marked SYNTHESIZED.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MINI_ROUND_THREE` (unanimous-correct MR2) |
| Synthesizer | Leader produces a coherent synthesized answer for each tx |
| Evaluators | All 9 non-leader validators: `approve_all` |
| Votes received | 9 (all) → quorum 6 satisfied |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SYNTHESIZED |

---

### 2 — `partial_tx_eligibility`

**What it tests:** the leader skips ineligible transactions and synthesizes
only those that MR2 marked ready; final answers correctly mix SYNTHESIZED and
SKIPPED depending on each transaction's MR2 outcome.

| Dimension | Value |
|---|---|
| MR2 precondition | tx-A: `READY_FOR_MR3`; tx-B: `INSUFFICIENT_CORRECT_ANSWERS` (4 producers return wrong answer); tx-C: `NON_RELATED` (labeled `non_related` by MR1 committee) |
| Synthesizer | Leader synthesizes only tx-A |
| Evaluators | All 9 approve |
| MR3 finalizes | Yes |
| Final answer statuses | tx-A → SYNTHESIZED; tx-B, tx-C → SKIPPED |

**Design note:** achieving tx-C `NON_RELATED` requires that the MR1 committee
assigns `non_related` as the dominant label for that transaction. This can be
done by giving tx-C the label `["non_related"]` in the fixture, which
excludes it from MR2 processing entirely.

---

### 3 — `no_eligible_transactions`

**What it tests:** when every transaction from MR2 is either insufficient or
non-related, the leader broadcasts an empty proposal (zero synthesized answers),
validators approve trivially (no evaluation calls are needed), and the block
finalizes with every final answer as SKIPPED.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `INSUFFICIENT_CORRECT_ANSWERS` |
| Synthesizer | Leader produces empty `SynthesizedAnswers` list |
| Evaluators | All 9 approve (vacuously — `EvaluateSynthesisBatch` is called with an empty slice) |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SKIPPED |

---

### 4 — `minority_evaluators_reject`

**What it tests:** a minority of validators find the synthesis unacceptable and
silently abstain. Quorum is still reached by the honest approving majority so
the block finalizes normally. Protocol safety: individual rejection cannot block
consensus as long as enough honest validators approve.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesizer | Leader synthesizes all 3 |
| Evaluators | member-7, member-8, member-9: `reject_all`; all others: `approve_all` |
| Votes received | 6 (leader + member-1..6 approve; member-7..9 abstain) → exactly quorum 6 |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SYNTHESIZED |

---

### 5 — `evaluator_execution_error`

**What it tests:** one validator's evaluation agent throws an error during
`EvaluateSynthesisBatch`. The validator must send no partial vote and must not
crash the protocol. Remaining honest validators still reach quorum.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesizer | Leader synthesizes all 3 |
| Evaluators | member-1: `execution_error` (errorTxHash = target tx); all others: `approve_all` |
| Vote delivery order | member-1 placed last so the first 7 valid votes arrive before the failing validator's slot |
| Votes received | 8 valid (member-1 abstains due to error) → quorum 6 satisfied |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SYNTHESIZED |

---

### 6 — `byzantine_synthesis_vote`

**What it tests:** a committee member approves the synthesis internally but its
vote message is corrupted at the network boundary (invalid signature). The
leader must reject the tampered vote and still achieve quorum from the remaining
honest validators.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesizer | Leader synthesizes all 3 |
| Evaluators | All 9: `approve_all` |
| Synthesis vote faults | `{ "senderRole": "member-1", "type": "invalid_signature" }` |
| Votes accepted by leader | 8 valid (member-1's vote rejected) → quorum 6 satisfied |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SYNTHESIZED |

**Implementation note:** the `invalid_signature` fault replaces the vote's
`Signature` field with a signature produced by a different key pair, exactly as
the MR2 `byzantine_signed_classification_vote` pattern does for classification
votes.

---

### 7 — `shuffled_synthesis_vote_arrival`

**What it tests:** votes arrive in non-sequential delivery order (e.g.,
member-9 before member-1). Content stays valid. Quorum must still be reached
and the finalized block must match the unanimous-correct baseline, verifying
that vote-collection is ordering-independent.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesizer | Leader synthesizes all 3 |
| Evaluators | All 9: `approve_all` |
| `synthesizerVoteOrder` | `["leader","member-9","member-8","member-7","member-6","member-5","member-4","member-3","member-2","member-1"]` |
| MR3 finalizes | Yes |
| Final answer statuses | All 3 SYNTHESIZED |

---

### 8 — `tampered_synthesis_proposal_hash`

**What it tests:** the leader's broadcast is mutated at the network boundary so
that the `SynthesisBlockHash` field no longer matches the proposal contents.
Every non-leader validator rejects the message with
`ErrSynthesisBlockHashMismatch` and sends no vote. The leader receives zero
votes and the MR3 block is never finalized.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesis proposal faults | `{ "type": "mutate_proposal_hash" }` |
| Votes received by leader | 0 |
| MR3 finalizes | **No** |
| `mr3ErrorContains` | `"miniround3: synthesis block hash mismatch"` |
| `mr3FinalizedNodes` | 0 |

**Implementation note:** this scenario requires the network stub to support a
`synthesisProposalFaults` mutation applied to the `ProposedSynthesisConsensusMessage`
broadcast. The leader's local round-state already holds the valid proposal; only
the outgoing broadcast is corrupted.

---

### 9 — `synthesis_quorum_not_reached`

**What it tests:** too many validators reject the synthesis, reducing available
votes below the quorum threshold. After a timeout is injected (analogous to
`classification_quorum_timeout` in MR2), the round terminates without
finalizing MR3.

| Dimension | Value |
|---|---|
| MR2 precondition | All 3 txs `READY_FOR_MR3` |
| Synthesizer | Leader synthesizes all 3 |
| Evaluators | member-2..9 (8 validators): `reject_all`; member-1: `approve_all` |
| Votes received | 1 (only member-1) → below quorum 6 |
| Timeout injection | Fired after `len(votes) == 1` is observed (mirrors `classification_quorum_timeout`) |
| MR3 finalizes | **No** |
| `mr3ErrorContains` | timeout / no quorum sentinel (to be defined during implementation) |
| `mr3FinalizedNodes` | 0 |

**Implementation dependency:** this scenario requires adding timeout-step
injection to the MR3 test runner, mirroring `scheduleScenarioTimeout` in
`miniroundtwo_test.go`. The `data.Step` enum and the MR3 event loop need a
`SynthesisVoteCollectionStep` constant before this scenario can be wired.

---

## Running the suite

```sh
go test -tags integration ./integrationtests \
  -run TestMiniRoundOneToMiniRoundThreeScenarios
```

---

## Adding a scenario

1. Copy the closest existing `scenario.json` into a directory whose name
   matches its `name` field.
2. Add or modify only the network behavior or adversarial condition under test.
3. Define explicit expected finalization, voter lists, and final answer statuses.
4. Run the suite and inspect assertion failures before updating any expectation.

Keep live-model observations and performance experiments out of these fixtures;
their permanent summaries belong under `testresults/`.

---

## Prompt for generating diverse mocked fixture data

Use the following prompt with separate Claude instances (one per scenario) to
produce non-identical, realistic coding-theory content for the `transactions`,
`executors`, and `synthesizer` fields. Each instance should produce output for
**exactly one** assigned scenario number.

---

> **System context:**
> You are generating test fixture data for a blockchain consensus protocol
> integration test suite. The protocol processes coding-theory questions
> submitted as transactions. Validators label, answer, and then synthesize
> responses. Your output must be valid JSON fragments that will be inserted
> into a `scenario.json` file. Do not wrap the JSON in markdown fences.
>
> **Your scenario number:** `<N>` (replace with 1–9 before sending)
>
> **Task:** Generate the following JSON objects for scenario `<N>`. Every value
> must be original — do not reuse prompts or answers from any other scenario.
>
> ---
>
> **1. Three transaction entries** (`transactions` array).
>
> Rules:
> - Each transaction has a unique `"role"`: `"control-before"`, `"scenario-target"`, `"control-after"`.
> - `"txHash"` must follow the pattern `"scenario-<NN>-<role>"` where `<NN>` is
>   the zero-padded scenario number (e.g., `"scenario-03-control-before"`).
> - `"prompt"` must be a concise, technically meaningful coding question (15–30
>   words). Topics must differ across the three transactions and must vary across
>   scenarios. Examples of good topics: memory safety in Rust, TCP backpressure,
>   CI/CD pipeline design, database index selection, WebAssembly sandboxing,
>   React reconciliation, Kubernetes pod scheduling.
> - `"labels"` must be 1–3 items drawn **exclusively** from this set:
>   `"systems_programming"`, `"web_front_end"`, `"back_end_with_apis"`,
>   `"ml_ai_engineering"`, `"data_engineering"`, `"dev_ops"`, `"security"`,
>   `"mobile_dev"`, `"test_engineering_and_qa_automation"`,
>   `"blockchain_engineering"`, `"cloud_engineering"`, `"databases"`.
>   Labels must match the technical content of the prompt.
> - Keep `"sender"`, `"receiver"`, `"nonce"`, `"transferredValue"`,
>   `"thinkingMode"`, `"userOutputDimension"` at the same fixed values used in
>   the existing MR2 fixtures (`"moa-chain"`, `0`, `0`, `"fast"`, `"short"`).
>   Use `"alice"`, `"bob"`, `"carol"` as senders in order.
>   Assign `"tip"` values `90`, `80`, `70` and `"timestamp"` values `1`, `2`, `3`
>   in order.
>
> **2. Executor profiles** (`executors` array).
>
> Provide a `"default"` executor with one answer per transaction hash.
> Answers must be 1–2 sentences, technically accurate, and vary in phrasing from
> any existing fixture. If the scenario being generated involves multiple executor
> roles (scenarios 2, 4, 5, 9 from the table above have `INSUFFICIENT` or
> `NON_RELATED` preconditions), also generate a second executor role whose answer
> for the `"scenario-target"` transaction is subtly but clearly wrong (not a
> hallucination — just factually incorrect in one claim, e.g., wrong causation
> or inverted logic).
>
> **3. Synthesizer output** (`synthesizer.answers` object).
>
> For each transaction that is `READY_FOR_MINI_ROUND_THREE` in the scenario,
> provide a synthesized answer (2–4 sentences). The synthesized answer should
> read as a coherent distillation of the executor's correct answer — not a
> verbatim copy, but a more structured or slightly expanded version that
> references the key technical point. For SKIPPED transactions, omit the entry.
>
> **Output format:** Return a single JSON object with three keys:
> `"transactions"`, `"executors"`, `"synthesizer"`. Nothing else.

---

### How to use this prompt

1. Make 9 copies of the prompt above.
2. Replace `<N>` with `1` through `9` in each copy.
3. Send each copy to a separate Claude instance (or use the sub-agent spawning
   capability described in the project notes).
4. Collect the 9 JSON responses and merge each into the corresponding
   `scenario.json` skeleton, filling in the `network`, `judges`, `delivery`,
   and `expected` fields manually based on the scenario table above.

The prompts are intentionally scoped so each agent has no knowledge of what the
others produce, ensuring answer and phrasing diversity across the fixture set.
