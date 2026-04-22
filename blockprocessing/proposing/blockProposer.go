package proposing

import (
	"moa-chain/data"
)

type blockCreator struct {
}

// NewBlockCreator creates a new block creator
func NewBlockCreator() *blockCreator {
	return &blockCreator{}
}

// ProposeBlock proposes a block
func (bc *blockCreator) ProposeBlock() (*data.Block, error) {
	// get current state of the chain
	// set nonce, round, mini round, epoch, previous header hash, previous root hash

	// initialize an account adapter, select transactions from mempool

	// label each transaction in case it is not already labeled from mempool, call the labeler

	// create the block body

	// validate new block body

	// update block header with new hash and new root hash

	// return proposed block

	return nil, nil
}
