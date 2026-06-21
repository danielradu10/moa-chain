package blockFinalizer

import (
	"moa-chain/data"
)

// BlockFinalizer defines what should be done when a block is finalized in different mini-rounds.
type BlockFinalizer interface {
	FinalizeBlockMROne(block *data.BlockOnChain) error
	GetFinalizedBlockInMROne(key data.RoundKey) (*data.BlockOnChain, error)
}
