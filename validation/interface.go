package validation

import (
	"moa-chain/data"
)

// BlockProcessor defines what a BlockProcessor should do
type BlockProcessor interface {
	ProcessBlock(block *data.Block) error
}

// TxProcessor defines what a TxProcessor should do
type TxProcessor interface {
	ProcessTransaction(tx data.Transaction) error
}
