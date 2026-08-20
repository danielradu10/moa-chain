package explorer

import (
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/txpipeline"
	"moa-chain/tracker"
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
}

func (nv *NodeView) ChainLength() uint64               { return nv.Chain.Len() }
func (nv *NodeView) CurrentRound() (uint64, error)     { return nv.BlockchainState.CurrentRound() }
func (nv *NodeView) CurrentMiniRound() (uint64, error) { return nv.BlockchainState.CurrentMiniRound() }
func (nv *NodeView) CurrentEpoch() (uint64, error)     { return nv.BlockchainState.CurrentEpoch() }
func (nv *NodeView) GetTransactionStatus(hash string) (tracker.TxStatus, bool) {
	return nv.TxTracker.GetStatus(hash)
}
