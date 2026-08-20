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

---

## Package layout

```
explorer/
  node_view.go          — NodeView: aggregates raw state references; implements NodeFacade
  models.go             — API response structs (view-model / DTO layer)

  controllers/
    server.go           — HTTP server, mux wiring, route registration via RequestHandler
    request_handler.go  — RequestHandler: pairs HTTP method + path + handler
    handlers.go         — HTTP handler functions (thin: parse → service call → write JSON)

  service/
    node_facade.go      — NodeFacade interface: what the service needs from the node
    service.go          — ExplorerService: query logic, builds view models
```

Future files as tasks are completed:

```
  service/
    tx_resolver.go      — multi-source tx status (TxTracker + mempool + store + chain)
    round_resolver.go   — round details from blockFinalizer + chain

  observer.go           — RoundObserver: atomic snapshot of live round progress
  tx_tracker.go         — TxTracker: transaction lifecycle events + SSE fan-out
  sse.go                — SSE writer helper
```

### Design decisions

- **HTTP router**: `net/http` stdlib (Go 1.22+). Method+path routing via `"GET /api/v1/health"` patterns; path params via `r.PathValue("key")`. No external dependency.
- **`NodeFacade` interface**: `ExplorerService` depends on `NodeFacade`, not on `*NodeView` directly. `NodeView` implements it. Tests substitute a stub — no real components needed.
- **`RequestHandler` pattern**: each route is a `{httpMethod, path, handler}` triple collected in `routes()`. One place to see all registered endpoints.

The view-model layer (`models.go`) is a deliberate boundary: internal structs
(`data.BlockOnChain`, `data.AnswerClassificationVote`, etc.) are never serialised
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

### Note: transaction hash is caller-supplied, not computed by the node

There is no canonical `ComputeTxHash` function in production code. The only
implementation lives in the integration tests (`computeTestTxHash`) and hashes
`sender + nonce + prompt + tip + timestamp` with SHA-256 and a domain separator.

`TxInterceptor.validate()` rejects any transaction whose hash is empty
(`ErrEmptyTxHash`), so the hash must be set by the caller **before** `Submit`
is called. Today this is fine: the explorer looks up transactions by the hash
already present on the object.

When `SubmitTransaction` is eventually implemented, the server will need to:
1. Compute the hash from the submitted fields and set it on the transaction
   before passing it to the interceptor.
2. Return the hash immediately as the submission receipt so the client can
   track the lifecycle via `GET /api/v1/transactions/{hash}` and SSE.

The exact field set for the canonical hash is not yet formally specified. The
test uses `sender + nonce + prompt + tip + timestamp`. Before submission is
built, the following should be decided:
- Whether `timestamp` belongs (client-controlled, weaker collision resistance).
- Whether `receiver` and `transferredValue` belong (they affect economic outcomes).
- Which package owns the canonical function (`mempool` or `txpipeline`).

### Why TxTracker must come before the transaction endpoint

`mempool.GetPendingTransactions()` only sees transactions that have **completed**
preprocessing. Anything earlier — a transaction currently being broadcast or
actively running LLM inference — is invisible to the mempool. Without TxTracker,
SUBMITTED and PREPROCESSING states cannot be reported, and the transaction
endpoint would silently return nothing for in-flight transactions. TxTracker
must therefore be built before the transaction lookup endpoint, not after.

---

## Implementation tasks

| # | Task | What changes | Status |
|---|---|---|---|
| 1 | README | `explorer/README.md` | ✓ done |
| 2 | Mempool extension | Add `GetPendingTransactions()` to `mempool.Mempool` interface + implementation | ✓ done |
| 3 | `explorer` package skeleton | `NodeView`, `NodeFacade`, `ExplorerService`, `controllers/`, `service/`, health endpoint (`GET /api/v1/health`) | ✓ done |
| 4 | `TxTracker` | Standalone lifecycle tracker; callback hooks into `TxInterceptor` + `TxPreprocessor`; covers SUBMITTED → PREPROCESSING → PENDING | |
| 5 | Block + round endpoints | `GET /api/v1/blocks/{hash}`, `GET /api/v1/rounds/{round}` | |
| 6 | Transaction lookup | `GET /api/v1/transactions/{hash}` — full status from TxTracker + mempool + chain | |
| 7 | Live round state | `RoundObserver`, wire into `consensus/round.go`, `GET /api/v1/live/round` | |
| 8 | SSE tx lifecycle | Wire TxTracker event fan-out to `GET /api/v1/transactions/{hash}/events` | |

Each task is independently reviewable and leaves tests + the existing suite green.
