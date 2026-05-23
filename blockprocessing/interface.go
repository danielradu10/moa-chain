package blockprocessing

import (
	"moa-chain/data"
)

// BlockCreator defines the interface of a block creator
type BlockCreator interface {
	ProposeBlock() (*data.Block, error)
}

// BlockProcessor defines what a BlockProcessor should do
type BlockProcessor interface {
	ValidateBlock(block *data.Block) ([]byte, error)
}
