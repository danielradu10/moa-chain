package explorer

import (
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/tracker"
	"moa-chain/txpipeline"
	"moa-chain/validators"
)

// NodeView aggregates the read-only state references the explorer needs.
// All fields are set once at wiring time and never mutated after that.
// Every referenced component is already thread-safe (RWMutex or atomic),
// so handlers can read concurrently without additional locking.
type NodeView struct {
	Chain             chain.Chain
	BlockchainState   state.BlockchainState
	BlockFinalizer    blockFinalizer.BlockFinalizer
	ValidatorRegistry validators.ValidatorRegistry
	Store             txpipeline.PrecomputedStore
	Mempool           mempool.Mempool
	TxTracker         *tracker.TxTracker
	RoundTracker      *tracker.RoundTracker
}

// ChainLength returns the number of finalized blocks on the chain.
func (nv *NodeView) ChainLength() uint64 { return nv.Chain.Len() }

// CurrentRound returns the round number the node is currently processing.
func (nv *NodeView) CurrentRound() (uint64, error) { return nv.BlockchainState.CurrentRound() }

// CurrentMiniRound returns the mini-round number within the current round.
func (nv *NodeView) CurrentMiniRound() (uint64, error) {
	return nv.BlockchainState.CurrentMiniRound()
}

// CurrentEpoch returns the current epoch number.
func (nv *NodeView) CurrentEpoch() (uint64, error) { return nv.BlockchainState.CurrentEpoch() }

// GetTransactionStatus returns the lifecycle status of a transaction by hash.
func (nv *NodeView) GetTransactionStatus(hash string) (tracker.TxStatus, bool) {
	return nv.TxTracker.GetStatus(hash)
}

// AllBlocks returns all finalized blocks on the chain, oldest first.
func (nv *NodeView) AllBlocks() []*data.BlockOnChain { return nv.Chain.Blocks() }

// GetRound returns the tracked state for a specific epoch/round pair.
func (nv *NodeView) GetRound(epoch, round uint64) (tracker.RoundEntry, bool) {
	return nv.RoundTracker.GetRound(epoch, round)
}
