# Transaction Pipeline

## Goal

Move labeling and answering out of the consensus hot path.

In the original design, `ExecuteBlockBodyMiniRoundOne` calls `LabelBatch` and
`ExecuteBlockBodyMiniRoundTwo` calls `AnswerBatch` — both blocking, synchronous
LLM calls that happen while a consensus round is in progress. Round latency
therefore includes model inference time, which is variable and potentially long.

The transaction pipeline solves this by doing the expensive work ahead of time.
Transactions are labeled and answered before they reach the mempool. By the time
a round starts, every transaction it can select already has its node-local labels
and answer stored in a cache. MR1 and MR2 become a cache lookup, not a model
call, making round execution depend only on network message latency.

## Components

### TxInterceptor

The single public entry point for receiving a transaction on a node. Responsible
only for fast, synchronous admission logic:

- Basic structural validation (non-nil fields, valid hash, etc.)
- Deduplication by transaction hash using a local seen-set

If the transaction passes admission, the interceptor does two things
independently:

1. Enqueues the transaction for preprocessing.
2. Broadcasts the raw transaction to all peer nodes.

The interceptor exposes two methods with different broadcast semantics:

- `Submit(tx)` — called by a client or a test. On success, the raw transaction
  is broadcast to peers so they can independently preprocess it.
- `Receive(tx)` — called by a peer node's broadcaster. Runs the same admission
  logic but does not re-broadcast, preventing message loops.

### TxPreprocessor

Processes transactions asynchronously from a queue. For each transaction, it
launches two concurrent operations:

- `LabelBatch([tx])` — produces the node-local subdomain labels.
- `AnswerBatch([tx])` — produces the node-local answer to the transaction
  prompt.

When both complete, the preprocessor stores the results in the
`PrecomputedStore` and calls `mempool.AddTransaction`. A transaction becomes
eligible for inclusion in a block only at that moment.

Labeling and answering are node-local and independent. Different nodes will
produce different answers — that is the entire point of consensus. Only the raw
transaction is propagated across the network; labels and answers are never
shared.

### PrecomputedStore

A node-local, thread-safe cache keyed by transaction hash. Stores:

- Labels `[]string` — the subdomain classification produced by `LabelBatch`.
- Answer `string` — the model response produced by `AnswerBatch`.

The store is write-once per entry: the preprocessor stores on completion, and
MR1/MR2 only read. Entries are removed after a block containing the transaction
is finalized and appended to the chain.

### TxBroadcaster

Delivers a raw transaction to every peer node's `TxInterceptor.Receive`. Uses
a registry of peer callbacks (one per node), not the consensus event inbox, so
transaction propagation is completely decoupled from consensus messaging.

## Transaction Lifecycle

```
Client / test
     |
     | Submit(tx)
     v
TxInterceptor
  - validate
  - deduplicate
     |
     +---> TxBroadcaster -----------> peer TxInterceptor.Receive()
     |                                  (same pipeline, no re-broadcast)
     |
     | enqueue
     v
TxPreprocessor
  - LabelBatch([tx])   \
                        +--> both concurrent
  - AnswerBatch([tx])  /
     |
     | on both complete:
     +---> PrecomputedStore.StoreLabels(txHash, labels)
     +---> PrecomputedStore.StoreAnswer(txHash, answer)
     +---> mempool.AddTransaction(tx)
```

From this point the transaction is visible to block selection. When a round
starts and the proposer calls `SelectTransactions`, every returned transaction
is guaranteed to have its labels and answer in the local store.

## Contract with Consensus

### Mini-Round One

`ExecuteBlockBodyMiniRoundOne` replaces its `LabelBatch` call with a
`PrecomputedStore.GetLabels` lookup per transaction. If a transaction is in the
proposed block but its labels are absent from the store, execution returns an
error — this indicates the transaction entered the mempool without going through
the pipeline, which is a programming error in test setup.

### Mini-Round Two

`ExecuteBlockBodyMiniRoundTwo` replaces its `AnswerBatch` call with a
`PrecomputedStore.GetAnswer` lookup per transaction. Token counting
(`CountTokensFromAnswer`) is applied to the retrieved answer as before. The rest
of the MR2 flow — answer evidence, classification, judging, finalization — is
unchanged.

## Known Limitations

**Nonce ordering.** The preprocessor processes transactions concurrently. If two
transactions from the same sender are enqueued, their `AnswerBatch` calls race.
Whichever finishes last wins the `mempool.AddTransaction` call, but the mempool
enforces nonce order — adding nonce 1 before nonce 0 will fail. The current
implementation does not include a per-sender ordering gate. For the PoC all
tests use at most one transaction per sender, so this is not triggered. A
per-sender queue with a hold-until-predecessor-added mechanism is the correct
fix if multi-nonce senders become a test case.

**Batching.** The original body executor issued one `LabelBatch` and one
`AnswerBatch` call per block, amortizing HTTP overhead across all transactions.
The preprocessor issues one call per transaction as it arrives. For the PoC this
is correct; a time-windowed batcher that collects arriving transactions and
flushes them together is an optimization left for later.

**Cache eviction.** `PrecomputedStore` entries are not currently removed after
finalization. For long-running nodes the store will grow unbounded. Eviction
should be triggered by the post-round cleanup that already calls
`mempool.RemoveTransactions` after a block is appended to the chain.
