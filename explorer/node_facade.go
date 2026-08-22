package explorer

import (
	"moa-chain/data"
	"moa-chain/tracker"
)

// NodeFacade is the read-only view of the node exposed to the explorer layer.
// NodeView implements it; tests substitute a stub.
type NodeFacade interface {
	// ChainLength returns the number of finalized blocks on the chain.
	ChainLength() uint64
	// CurrentRound returns the round number the node is currently processing.
	CurrentRound() (uint64, error)
	// CurrentMiniRound returns the mini-round number within the current round.
	CurrentMiniRound() (uint64, error)
	// CurrentEpoch returns the current epoch number.
	CurrentEpoch() (uint64, error)
	// GetTransactionStatus returns the lifecycle status of a transaction by its raw hash bytes.
	GetTransactionStatus(txHash []byte) (tracker.TxStatus, bool)
	// GetPendingTransactions returns a snapshot of all transactions currently in the mempool.
	GetPendingTransactions() []data.Transaction
	// GetPendingTransaction returns a transaction currently in the mempool by its raw hash bytes.
	GetPendingTransaction(txHash []byte) (data.Transaction, bool)
	// GetPrecomputedLabels returns the domain labels assigned to a transaction during preprocessing.
	GetPrecomputedLabels(txHash []byte) ([]string, bool)
	// GetPrecomputedAnswer returns the local model answer for a transaction from the precomputed store.
	GetPrecomputedAnswer(txHash []byte) (string, bool)
	// AllBlocks returns all finalized blocks on the chain, oldest first.
	AllBlocks() []*data.BlockOnChain
	// GetRound returns the tracked state for a specific epoch/round pair.
	GetRound(epoch, round uint64) (tracker.RoundEntry, bool)
}
