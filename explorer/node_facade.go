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
	// GetTransactionStatus returns the lifecycle status of a transaction by hash.
	GetTransactionStatus(hash string) (tracker.TxStatus, bool)
	// AllBlocks returns all finalized blocks on the chain, oldest first.
	AllBlocks() []*data.BlockOnChain
	// GetRound returns the tracked state for a specific epoch/round pair.
	GetRound(epoch, round uint64) (tracker.RoundEntry, bool)
}
