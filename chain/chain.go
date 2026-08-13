package chain

import (
	"bytes"
	"sync"

	"moa-chain/data"
)

// Chain is the interface for the canonical blockchain.
// Each entry is the canonical output of one full round: the MR3 block if MR3
// ran, the MR2 block if MR3 was skipped, or the MR1 block if MR2 was also
// skipped. Blocks are appended in round order and never removed.
type Chain interface {
	// Append adds the canonical block for the just-completed round.
	Append(block *data.BlockOnChain) error
	// Head returns the most recently appended block.
	// Returns ErrEmptyChain if no blocks have been appended yet.
	Head() (*data.BlockOnChain, error)
	// Len returns the number of blocks currently stored in the chain.
	Len() uint64
}

type chain struct {
	mu     sync.RWMutex
	blocks []*data.BlockOnChain
}

// NewChain creates an empty chain ready to accept the first round output.
func NewChain() Chain {
	return &chain{
		blocks: make([]*data.BlockOnChain, 0),
	}
}

// Append adds the canonical block for the just-completed round.
// If the chain is non-empty, the block's round must be exactly head.Round + 1.
func (c *chain) Append(block *data.BlockOnChain) error {
	if block == nil {
		return ErrNilBlock
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.blocks) > 0 {
		head := c.blocks[len(c.blocks)-1]
		if block.Header.Round != head.Header.Round+1 {
			return ErrNonContiguousBlock
		}
		if !bytes.Equal(block.Header.PreviousHash, head.Header.HeaderHash) {
			return ErrPreviousHashMismatch
		}
	}

	c.blocks = append(c.blocks, block)
	return nil
}

// Head returns the most recently appended block.
func (c *chain) Head() (*data.BlockOnChain, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.blocks) == 0 {
		return nil, ErrEmptyChain
	}

	return c.blocks[len(c.blocks)-1], nil
}

// Len returns the number of blocks currently stored in the chain.
func (c *chain) Len() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return uint64(len(c.blocks))
}
