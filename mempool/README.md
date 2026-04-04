# Mempool

The `mempool` package stores candidate transactions and selects the set that should compete for inclusion in the next block.

In the current design, a transaction models a prompt execution request. The mempool is responsible for:

- precomputing selection metadata when a transaction enters the pool
- grouping transactions by sender
- maintaining deterministic per-sender ordering
- selecting a block candidate set under nonce, balance, and block-consumption constraints

## Current Surface

The package currently revolves around two operations:

- `AddTransaction(transaction Transaction) error`
- `SelectTransactions(accountsState state.AccountsState, comparator txHeapComparator) []Transaction`

Other lifecycle operations such as removal and cleanup are not implemented yet.

## Transaction Model

The `Transaction` interface contains the usual chain fields such as:

- `Nonce`
- `Sender`
- `Receiver`
- `TransferredValue`
- `Tip`
- `Timestamp`
- `TxHash`

It also contains prompt-specific fields used by the mempool scoring logic:

- `Prompt`
- `UserOutputDimension`
- `ThinkingMode`
- `EstimatedConsumption`
- `EstimatedFee`
- `EstimatedScore`

## Scoring

When a transaction is added, the mempool precomputes its estimated consumption and score.

Current formula:

```text
score = tip / estimatedConsumption
```

Current estimated consumption:

```text
estimatedConsumption =
    promptTokens
  + thinkingModeEstimatedTokens
  + userOutputDimensionEstimatedTokens
```

Where:

- `promptTokens` comes from `tiktoken` using `cl100k_base`
- `thinkingModeEstimatedTokens` comes from `tokensConfig`
- `userOutputDimensionEstimatedTokens` comes from `tokensConfig`

This gives the selector a simple value-density heuristic: higher tip and lower expected consumption produce a better score.

## Insertion Rules

Transactions are stored in two structures:

- `transactionsByHash`: cache keyed by `txHash`
- `senders`: map of sender address to a sorted `txList`

Duplicate hashes are ignored. If a transaction with the same `txHash` already exists, the second insertion is a no-op.

Inside each sender list, transactions are sorted deterministically by:

1. increasing nonce
2. increasing estimated consumption
3. increasing `txHash`

This ordering matters because selection progresses sender-by-sender through these already-sorted lists.

## Selection Algorithm

`SelectTransactions` works on a snapshot of the mempool to avoid concurrent interference with ongoing inserts.

At a high level:

1. Snapshot the sender map.
2. Create a heap with the first transaction of each sender.
3. Repeatedly pop the best heap item.
4. Check whether the sender or transaction is still valid in the current selection session.
5. If valid, select it and update the sender's virtual state.
6. Push the sender's next transaction back into the heap.
7. Stop when the next selected transaction would exceed the block consumption cap.

The default heap comparator is `isTransactionMoreValuable`, which orders candidates by:

1. descending estimated score
2. increasing estimated consumption
3. increasing `txHash`

This means selection is globally competitive across senders, but sequential within each sender.

## Virtual Selection State

Selection uses a `selectionSession` with per-sender virtual records. A virtual record tracks:

- initial on-chain nonce
- initial on-chain balance
- current virtual nonce after already-selected transactions
- accumulated transferred value

This allows the selector to simulate sender progression without mutating chain state.

### Sender-level skip rules

A sender is skipped for the entire session if its first competing transaction starts with an initial nonce gap. Because sender transactions are already sorted by nonce, a gap at the front means the rest of that sender list cannot currently become valid either.

A sender is also skipped for the current candidate if the transaction nonce is higher than `currentVirtualNonce + 1`.

### Transaction-level skip rules

A transaction is skipped if:

- its nonce is below the sender's initial on-chain nonce
- its nonce is below the next expected virtual nonce
- its transferred value exceeds the sender's initial balance
- its transferred value, when added to already-selected value from the same sender, would exceed the sender's initial balance

When a transaction is selected, the virtual record is updated by:

- moving the sender nonce forward
- accumulating transferred value

## Block Consumption Limit

Selection stops when adding the next best transaction would exceed the package-level constant:

```go
const maxBlockConsumption = 10000
```

This is a hard stop in the current implementation. Timeout-based exits, max-transaction limits, and config-driven limits are still future work.

## Concurrency Model

The mempool uses an RW mutex:

- `AddTransaction` takes the write lock
- read-only accessors and snapshot creation take the read lock

Selection itself runs on a snapshot, which avoids holding the mempool lock during the full heap-selection process and prevents race-prone iteration over live structures.

## Tested Behavior

The current unit tests cover:

- nil transaction rejection
- duplicate transaction hash handling
- per-sender sorting by nonce, consumption, and hash
- score precomputation on insert
- token estimation from prompt plus config buckets
- cross-sender heap ordering
- block consumption cutoff
- sender skipping on nonce gaps
- transaction skipping on insufficient balance
- sequential selection of valid consecutive transactions from the same sender
- skipping later sender transactions when cumulative transferred value exceeds balance

## Current Limitations

- `tokensConfig` is required by the scoring logic, but its production wiring is not finished yet
- `estimatedFee` exists on the transaction model but is not computed or used in selection
- removal methods such as `RemoveTx` are not implemented
- cleanup methods for stale or invalid transactions are not implemented
- fairness and sender starvation are not addressed yet
- block-level stopping conditions are limited to `maxBlockConsumption`
- selector extensibility is limited by the current internal comparator type
- `cmd.MempoolConfig` exists but is still empty

## Design Notes

The current implementation favors deterministic behavior and simplicity:

- sender-local ordering is stable
- global competition is heap-based
- validation is driven by chain state plus virtual session state
- selection never mutates the live mempool

That makes the package a solid first version for experimentation, while leaving room for stronger fee models, fairness controls, cleanup paths, and configurable operating limits.

## Next Steps

The most obvious follow-ups are:

- wire a real config source for token buckets and block limits
- implement transaction removal by hash
- implement pool cleanup
- improve the scoring formula
- study starvation and fairness tradeoffs
- add timeout and selection-budget guards
- extend tests and add benchmarks
