package service

import (
	"moa-chain/data"
	"moa-chain/tracker"
)

// NodeFacade is the read interface the ExplorerService uses to query node state.
// NodeView implements it; tests can substitute a stub.
type NodeFacade interface {
	ChainLength() uint64
	CurrentRound() (uint64, error)
	CurrentMiniRound() (uint64, error)
	CurrentEpoch() (uint64, error)
	GetTransactionStatus(hash string) (tracker.TxStatus, bool)
	AllBlocks() []*data.BlockOnChain
	GetRound(epoch, round uint64) (tracker.RoundEntry, bool)
}
