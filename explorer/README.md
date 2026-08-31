# MoA Chain Explorer API

HTTP API that exposes the local simulator's state and accepts transaction
submission. Backed by an in-process node; best run via `cmd/localchain`.

---

## One explorer per chain

The explorer is **per-node**: it reads directly from one node's in-memory
state (its own `chain.Chain`, mempool, trackers, and consensus loop). Each
node has its own finalizer and its own chain instance — they are not shared.
After consensus completes, all honest nodes independently finalize the same
blocks in the same order, so any fully-synced node is a valid source for
finalized data.

Do **not** start an explorer server for every node. One server, wired to
one `NodeView`, is enough. For live consensus state (`/round/current`,
`/round/stream`) the data reflects that specific node's perspective, which
is expected and honest.

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
  node_facade.go        — NodeFacade interface (what ExplorerService needs from the node)
  models.go             — API request/response structs (view-model / DTO layer)
  round_hub.go          — RoundHub: fan-out for step events → SSE subscribers
  tx_hub.go             — TxHub: fan-out for tx status events → SSE subscribers; closes on FINALIZED

  controllers/
    server.go           — HTTP server, mux wiring, route registration via RequestHandler
    request_handler.go  — RequestHandler: pairs HTTP method + path + handler
    handlers.go         — HTTP handler functions (thin: parse → service call → write JSON)

  service/
    service.go          — ExplorerService: query + submit logic, builds view models
    tx_resolver.go      — multi-source tx status (TxTracker + mempool + store + chain)
    round_resolver.go   — round details from RoundTracker + chain

  testscommon/
    node_facade_stub.go — NodeFacadeStub for unit tests

cmd/
  localchain/
    main.go             — runnable simulator: N nodes, mock agent, explorer on node 0
```

The `tracker/` package lives at the repository root alongside `mempool/` and `txpipeline/`:

```
tracker/
  tx_tracker.go         — TxTracker: SUBMITTED → PREPROCESSING → PENDING → FINALIZED per tx
  round_tracker.go      — RoundTracker: MR1/MR2/MR3 block snapshots per round
```

`RoundTracker` is updated via `blockFinalizer.Callbacks` hooks (injected with `WithCallbacks`
at wiring time). `blockFinalizer` is cleared after chain append; `RoundTracker` is not —
it is the persistent store that `GET /api/v1/rounds/{round}` reads from.

### Design decisions

- **HTTP router**: `net/http` stdlib (Go 1.22+). Method+path routing via `"GET /api/v1/health"` patterns; path params via `r.PathValue("key")`. No external dependency.
- **`NodeFacade` interface**: `ExplorerService` depends on `NodeFacade`, not on `*NodeView` directly. `NodeView` implements it. Tests substitute a stub — no real components needed.
- **`RequestHandler` pattern**: each route is a `{httpMethod, path, handler}` triple collected in `routes()`. One place to see all registered endpoints.

The view-model layer (`models.go`) is a deliberate boundary: internal structs
(`data.BlockOnChain`, `data.AnswerClassificationVote`, etc.) are never serialised
directly. This lets internal types evolve without breaking the API contract.

---

## Running a local chain

`cmd/localchain` starts N validators, wires an explorer server to node 0, and
runs rounds continuously until you press Ctrl+C. It uses local mocked agents
when no agent config is supplied. Transactions submitted to the HTTP endpoint
are broadcast to all validators automatically.

When `--agent-config` is supplied, the local chain uses the real HTTP agents
from that experiment config and registers each validator under its
`validator_name`. The Make targets use the all-real heterogeneous brief-answer
config by default: run `make localchain-agents` first, then `make localchain` in
a second terminal.

```
go run ./cmd/localchain [--nodes N] [--start-round R] [--addr :PORT] [--agent-delay DURATION] [--mini-round-duration DURATION] [--mr1-vote-collection-deadline DURATION] [--mr2-classification-grace-period DURATION] [--mr3-approval-grace-period DURATION]
```

| Flag | Default | Description |
|---|---|---|
| `--nodes` | `10` | Number of validator nodes |
| `--start-round` | `2` | First round (genesis is round 1) |
| `--addr` | `:8080` | Explorer HTTP server address |
| `--agent-delay` | `5s` | Simulated LLM latency per agent call |
| `--mini-round-duration` | `15s` | Fixed slot per mini-round; `0` advances immediately |
| `--mr1-vote-collection-deadline` | `10s` | MR1 fallback deadline for collecting label votes; `0` waits for all committee members |
| `--mr2-classification-grace-period` | `10s` | MR2 post-quorum grace for collecting additional complete classification votes; `0` certifies immediately at quorum |
| `--mr3-approval-grace-period` | `10s` | MR3 post-quorum grace for collecting additional approval votes; `0` certifies immediately at quorum |
| `--agent-config` | empty | Experiment config supplying real agent endpoints and validator names; empty uses local mocks |

Example:

```sh
go run ./cmd/localchain --nodes 5 --start-round 2 --addr :9090
```

The binary pre-funds six known accounts (`alice`, `bob`, `carol`, `david`,
`eveline`, `frank`) with 1 000 000 balance each. Transactions with those
senders are accepted by block validation; any other sender will be rejected.

---

## Endpoints

### Submit transaction

```
POST /api/v1/transactions
Content-Type: application/json
```

Computes the canonical transaction hash (SHA-256 of
`"moa-chain-transaction-v1" + sender + nonce + prompt + tip + timestamp`),
builds a transaction, and submits it to node 0's tx interceptor which
broadcasts it to all validators.

Request body:

```json
{
  "sender": "alice",
  "prompt": "Implement a concurrent hash map in Go.",
  "nonce": 0,
  "tip": 50
}
```

Response `201 Created`:

```json
{
  "tx_hash": "<hex-encoded SHA-256 hash>",
  "timestamp": 1724078400000000000
}
```

Use `tx_hash` to track the transaction's lifecycle via
`GET /api/v1/transactions/{hash}` or subscribe to real-time updates via
`GET /api/v1/transactions/{hash}/events`.

Errors:
- `400 Bad Request` — `sender` or `prompt` is empty

### Live round state

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

### Canonical transaction hash

`txpipeline.ComputeTxHash(sender, nonce, prompt, tip, timestamp)` is the
single authoritative implementation. It computes SHA-256 of:

```
"moa-chain-transaction-v1" || sender || nonce || prompt || tip || timestamp
```

where strings are length-prefixed and integers are big-endian 8-byte. The
domain separator prevents cross-protocol hash collisions. `timestamp` is set
by the server at submission time (`time.Now().UnixNano()`), so the caller
does not control it.

`TxInterceptor.validate()` rejects any transaction whose hash is empty
(`ErrEmptyTxHash`). The `POST /api/v1/transactions` handler always sets the
hash before passing the transaction to the interceptor and returns it to the
client as the lookup key.

### Why TxTracker must come before the transaction endpoint

`mempool.GetPendingTransactions()` only sees transactions that have **completed**
preprocessing. Anything earlier — a transaction currently being broadcast or
actively running LLM inference — is invisible to the mempool. Without TxTracker,
SUBMITTED and PREPROCESSING states cannot be reported, and the transaction
endpoint would silently return nothing for in-flight transactions. TxTracker
must therefore be built before the transaction lookup endpoint, not after.

---

## Future improvements

### Per-validator data visibility

The explorer currently shows only **aggregated consensus outputs** — the quorum-agreed
result for each mini-round. Individual validator contributions are not stored in
`BlockOnChain` and are lost after vote aggregation.

It would be valuable to expose what each validator generated per mini-round:

| Mini-round | Per-validator data | Currently visible |
|---|---|---|
| MR1 | Each validator's label set per transaction | ✗ — only aggregate `SubdomainsFrequencies` |
| MR2 | Each validator's answer + consumption per transaction | ✗ — only `AggregatedExecutionResults` |
| MR3 | Each validator's synthesized answer | ✗ — only the canonical `FinalAnswers` |

Implementing this would require hooking into the vote collection phase of each
mini-round (before aggregation), not just the finalization step that
`blockFinalizer.Callbacks` exposes. Each `BlockVote` carries a signature and
block hash but not the individual label/answer payloads — those live in each
validator's local `BlockBodyExecutionResultMROne/Two/Three` structs and would
need to be captured at broadcast time.

---

## Implementation tasks

| # | Task | What changes | Status |
|---|---|---|---|
| 1 | README | `explorer/README.md` | ✓ done |
| 2 | Mempool extension | Add `GetPendingTransactions()` to `mempool.Mempool` interface + implementation | ✓ done |
| 3 | `explorer` package skeleton | `NodeView`, `NodeFacade`, `ExplorerService`, `controllers/`, `service/`, health endpoint (`GET /api/v1/health`) | ✓ done |
| 4 | `TxTracker` + `RoundTracker` | Lifecycle trackers in `tracker/`; callback hooks into pipeline and `blockFinalizer` | ✓ done |
| 5 | Block + round endpoints | `GET /api/v1/blocks/{hash}`, `GET /api/v1/rounds/{round}` | ✓ done |
| 6 | Transaction lookup | `GET /api/v1/transactions/{hash}` — full status from TxTracker + mempool + chain | ✓ done |
| 7 | Live round state | `RoundHub` (fan-out), `GET /api/v1/round/current`, `GET /api/v1/round/stream` (SSE) | ✓ done |
| 8 | SSE tx lifecycle | `TxHub` fan-out, `GET /api/v1/transactions/{hash}/events`, closes on FINALIZED | ✓ done |
| 9 | Transaction submission | `POST /api/v1/transactions`, `txpipeline.ComputeTxHash`, `NodeView.TxSubmitter` | ✓ done |
| 10 | `cmd/localchain` binary | N-node simulator with mock agent, explorer on node 0, SIGINT/SIGTERM shutdown | ✓ done |
| 11 | Explorer integration test | `TestExplorerIntegration` — snapshot + live SSE + POST tx + lifecycle to FINALIZED | ✓ done |

Each task is independently reviewable and leaves the existing test suite green.
