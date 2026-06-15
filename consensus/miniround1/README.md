# Mini-Round One

Mini-round one is the first implemented consensus step for finalizing a proposed block and the semantic subdomain evidence attached to that block.

At a high level, the round:

1. selects a deterministic consensus group and leader for the round
2. lets the leader propose a block built from the current mempool
3. has consensus-group validators validate the block and produce commit votes
4. lets the leader aggregate a quorum of votes into a certificate
5. finalizes the block on every validator that accepts the aggregated votes

## Main Components

- `handler`: coordinates consensus selection, block proposal, vote collection, aggregated-vote verification, and block finalization.
- `RoundHandler`: drives the round state machine and routes consensus messages to the mini-round handler.
- `RoundLoop`: consumes `RoundEvent` values from an inbox and reports handler errors through an error channel.
- `ValidatorRegistry`: selects the consensus group, exposes the group leader, and provides public keys for signature verification.
- `RoundState`: stores the proposed block, collected votes, and aggregated certificate for a specific `RoundKey`.

## Round Flow

### 1. Consensus Selection

Each validator calls `HandleConsensusSelection` at the start of the round.

The validator registry generates the consensus group from the blockchain state and `RoundKey`, then exposes the selected leader. If the local node is the leader, it moves to vote collection. Otherwise, it waits for the leader's proposal.

### 2. Block Proposal

The leader calls `ProposeBlockAndDomains` through the block creator.

The proposed block is saved in round state and broadcast to all validators. If the leader is part of the consensus group, it also signs its own block hash and subdomain hash, then stores its self-vote locally.

### 3. Block Validation And Voting

When a validator receives a proposed block, it verifies that:

- the message exists
- the block exists
- the sender is the selected leader
- the block validates through the block processor

Consensus-group validators then sign:

- the proposed block hash
- the hash of their generated `txHash -> labels` subdomain map

The validator sends the resulting `BlockVote` back to the leader.

### 4. Vote Aggregation

Only the selected leader can collect votes.

For each vote, the leader checks that:

- the signer is a registered validator
- the signer belongs to the current consensus group
- the block signature verifies against the proposed block hash

Once the leader has at least `2/3 + 1` votes from the consensus group, it builds `AggregatedVotes`, stores it as the round certificate, aggregates label frequencies from the included subdomain maps, finalizes the block locally, and broadcasts the aggregated votes.

### 5. Aggregated Vote Verification

Validators that receive aggregated votes verify that:

- the message came from the selected leader
- every block signature verifies against the proposed block hash
- every included subdomain map passes label validation
- every subdomain signature verifies against the subdomain hash

After verification, validators aggregate the included labels into `SubdomainsFrequencies` and finalize the same block.

## Finalized Artifact

Mini-round one finalizes a `BlockOnChain` containing:

- the proposed block
- aggregated subdomain frequencies derived from the accepted quorum certificate

The current implementation treats the first valid quorum certificate assembled by the leader as the source of truth for the finalized frequencies.

## Current Test Coverage

The integration tests cover:

- a round completing without loop errors
- all nodes finalizing the same empty block
- all nodes finalizing the same block with transactions
- all nodes finalizing the same block with agent-generated labels
- verifying that finalized frequencies can be produced by a valid quorum

Relevant fixtures live under `integrationtests/testData`.

Run the full test suite with:

```sh
go test ./...
```

## Known Limitations

- Timeouts are represented in the round handler, but there is no production timer wiring yet.
- The leader finalizes the first quorum it can assemble, so subdomain frequencies are deterministic for a chosen certificate but not necessarily deterministic across runs with different vote arrival order.
- Aggregated label frequencies are stored, but the finalized artifact does not yet persist the full certificate needed for later audit.
- Vote validation currently verifies block signatures before aggregation; stronger leader-side validation of subdomain payloads is still marked as future work.
- The protocol does not yet define a deterministic threshold rule for accepting canonical labels per transaction.
