package blockFinalizer

import (
	"moa-chain/data"
)

type BlockFinalizer interface {
	FinalizeBlock(block *data.BlockOnChain) error
}
