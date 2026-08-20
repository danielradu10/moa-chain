# MoA Chain Explorer API

Read-only HTTP API that exposes the local simulator's state to a future
frontend. No transaction submission in this version.

---

## What state already exists (and where)

| Component | Thread-safe | What it holds |
|---|---|---|
| `chain.Chain` | ✓ (RWMutex) | Canonical finalized blocks with full MR1→MR3 results |
| `state.BlockchainState` | ✓ (RWMutex) | Current round, mini-round, epoch |
| `blockFinalizer.BlockFinalizer` | ✓ (RWMutex) | Per-round MR1 / MR2 / MR3 blocks (cleared after chain append) |
| `validators.ValidatorRegistry` | reads only | Leader, committee, all registered validators |
| `txpipeline.PrecomputedStore` | ✓ (RWMutex) | Labels and LLM answer per tx hash |
| `mempool.Mempool` | ✓ (RWMutex) | Pending transactions (no GetAll today — needs extension) |
| `state.RoundState` | ✗ event-loop-owned | Live votes, proposals, certificates per round key |

### Thread-safety gap: RoundState

`RoundState` is intentionally not protected with a mutex — it is owned by a
single `RoundLoop` goroutine. Reading it from an HTTP handler goroutine is a
data race. The solution is a **push-based `RoundObserver`**: after each
relevant step, the round handler publishes an immutable `RoundSnapshot` via
`atomic.Value`. The HTTP handler reads from the snapshot; no mutex needed on
`RoundState` itself.

### Mempool gap: no GetAll

`mempool.Mempool` only exposes `SelectTransactions` (runs the full selection
algorithm). A `GetPendingTransactions() []data.Transaction` method will be
added to the interface and implementation to support the pending-tx view.

---

## Proposed package layout

```
explorer/
  node_view.go   — NodeView: aggregates all state references the explorer needs
  observer.go    — RoundObserver: thread-safe atomic snapshot of round progress
  tx_tracker.go  — TxTracker: transaction lifecycle events + SSE fan-out
  server.go      — HTTP server wiring, route registration
  handlers.go    — HTTP handler functions
  models.go      — API response structs (view-model / DTO layer)
  sse.go         — SSE writer helper
```

The view-model layer (`models.go`) is a deliberate boundary: internal structs
(data.BlockOnChain, data.AnswerClassificationVote, etc.) are never serialised
directly. This lets internal types evolve without breaking the API contract.

---

## Endpoints

### Live round state

```
GET /api/v1/live/round
```

Returns the current consensus state of this node (derived from
`BlockchainState`, `RoundObserver`, and `ValidatorRegistry`).

```json
{
  "epoch": 0,
  "round": 3,
  "mini_round": 1,
  "step": "StepCollectVotes",
  "leader": "validator-2",
  "committee": ["validator-1", "validator-2", "validator-3"],
  "votes_collected": 2,
  "quorum": 2,
  "status": "in_progress"
}
```

### Round details

```
GET /api/v1/rounds/{round}
```

Returns the finalized output of every mini-round for the given round number,
read from `blockFinalizer` and `chain`.

```json
{
  "round": 3,
  "epoch": 0,
  "status": "finalized",
  "mr1": {
    "proposer": "validator-2",
    "transactions": ["<txhash>"],
    "subdomains_frequency": { "databases": 5, "security": 3 }
  },
  "mr2": {
    "leader": "validator-1",
    "answer_classifications": [
      {
        "tx_hash": "<txhash>",
        "status": "READY_FOR_MINI_ROUND_THREE",
        "correct_count": 3
      }
    ]
  },
  "mr3": {
    "leader": "validator-3",
    "final_answers": [
      {
        "tx_hash": "<txhash>",
        "status": "SYNTHESIZED",
        "answer": "..."
      }
    ]
  }
}
```

### Block lookup

```
GET /api/v1/blocks/{hash}
```

Finds a block by its `HeaderHash` (hex-encoded) scanning `chain.Blocks()`.

```json
{
  "header_hash": "...",
  "previous_hash": "...",
  "round": 3,
  "epoch": 0,
  "transactions": [
    {
      "tx_hash": "...",
      "sender": "alice",
      "prompt": "Implement a concurrent hash map in Go.",
      "estimated_consumption": 120,
      "final_answer_status": "SYNTHESIZED"
    }
  ],
  "subdomains_frequency": { "databases": 5 },
  "total_final_answers": 1
}
```

### Transaction lookup

```
GET /api/v1/transactions/{hash}
```

Derives full transaction lifecycle from available state:
- `SUBMITTED` / `PREPROCESSING` — tracked by `TxTracker`
- `PENDING` — found in `mempool.GetPendingTransactions()`
- `FINALIZED` — found in `chain.Blocks()`

```json
{
  "tx_hash": "...",
  "sender": "alice",
  "prompt": "Implement a concurrent hash map in Go.",
  "nonce": 0,
  "estimated_consumption": 120,
  "status": "FINALIZED",
  "preprocessing": {
    "labels": ["databases", "systems_programming"],
    "answer": "Here is a concurrent hash map..."
  },
  "mr1": { "included_in_round": 3, "subdomains": ["databases"] },
  "mr2": {
    "status": "READY_FOR_MINI_ROUND_THREE",
    "correct_answers_count": 3
  },
  "mr3": {
    "final_answer": "Here is the synthesized answer...",
    "status": "SYNTHESIZED"
  },
  "finalized_block_hash": "..."
}
```

### Transaction SSE stream

```
GET /api/v1/transactions/{hash}/events
```

Server-Sent Events stream. The client connects once; the server pushes an
event each time the transaction's lifecycle status changes.

SSE is chosen over WebSocket because:
- The stream is one-directional (server → client only)
- SSE works over plain HTTP/1.1, needs no upgrade handshake
- Browsers support it natively; easy to consume from a React frontend

Event format:

```
event: tx_status
data: {"tx_hash":"...","status":"PREPROCESSING","timestamp":"2026-08-20T10:00:00Z"}

event: tx_status
data: {"tx_hash":"...","status":"PENDING","timestamp":"2026-08-20T10:00:01Z"}

event: tx_status
data: {"tx_hash":"...","status":"FINALIZED","block_hash":"...","round":3,"timestamp":"2026-08-20T10:00:05Z"}
```

---

## Transaction lifecycle states

```
SUBMITTED     → TxInterceptor.Submit called; broadcasting to peers
PREPROCESSING → TxPreprocessor has picked it up; LLM calls in flight
PENDING       → Labels + answer stored; tx added to mempool
FINALIZED     → Tx appears in a canonical chain block
```

`TxTracker` is a standalone component (not embedded in TxInterceptor or
TxPreprocessor). It receives lifecycle notifications via small callback hooks
injected at wiring time, keeping the pipeline components free of explorer
dependencies.

---

## Implementation tasks

| # | Task | What changes |
|---|---|---|
| 1 | This README | `explorer/README.md` |
| 2 | Mempool extension | Add `GetPendingTransactions()` to `mempool.Mempool` interface + implementation |
| 3 | `explorer` package skeleton | `NodeView`, `Server`, health endpoint (`GET /api/v1/health`) |
| 4 | Block + round endpoints | `GET /api/v1/blocks/{hash}`, `GET /api/v1/rounds/{round}` |
| 5 | Transaction lookup | `GET /api/v1/transactions/{hash}` (status derived from chain + mempool + store) |
| 6 | Live round state | `RoundObserver`, wire into `consensus/round.go`, `GET /api/v1/live/round` |
| 7 | SSE tx lifecycle | `TxTracker`, callback hooks in `TxInterceptor`/`TxPreprocessor`, `GET /api/v1/transactions/{hash}/events` |

Each task is independently reviewable and leaves tests + the existing suite green.
