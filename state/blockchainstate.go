package state

import (
	"errors"
	"sync"

	"moa-chain/chain"
	"moa-chain/data"
)

// ErrEmptyBlockchainState signals that CurrentBlockHeader was called before
// any block has been appended to the chain.
var ErrEmptyBlockchainState = errors.New("blockchain state is empty")

// blockchainState implements BlockchainState backed by a canonical Chain.
//
// CurrentBlockHeader always reflects the chain head (updated automatically
// when a block is appended). Round, MiniRound, and Epoch are updated
// explicitly via Update after each mini-round finalizes, since the chain
// head only changes at the end of a full round.
type blockchainState struct {
	mu               sync.RWMutex
	chain            chain.Chain
	currentRound     uint64
	currentMiniRound data.MiniRound
	currentEpoch     uint64
}

// NewBlockchainState creates a BlockchainState backed by the given Chain.
// Round, MiniRound, and Epoch are initialised to their zero values;
// call Update after the first mini-round finalizes.
func NewBlockchainState(c chain.Chain) BlockchainState {
	return &blockchainState{
		chain: c,
	}
}

// Update records the round, mini-round, and epoch of the mini-round that
// just finalized. Must be called by the round loop each time a mini-round
// completes (MR1, MR2, and MR3).
func (bs *blockchainState) Update(round uint64, miniRound data.MiniRound, epoch uint64) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	bs.currentRound = round
	bs.currentMiniRound = miniRound
	bs.currentEpoch = epoch
}

// CurrentBlockHeader returns the header of the most recently appended chain
// block. Returns ErrEmptyBlockchainState if no block has been appended yet.
func (bs *blockchainState) CurrentBlockHeader() (*data.ChainBlockHeader, error) {
	head, err := bs.chain.Head()
	if err != nil {
		return nil, ErrEmptyBlockchainState
	}

	return &head.Header, nil
}

// CurrentRound returns the round of the last finalized mini-round.
func (bs *blockchainState) CurrentRound() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.currentRound, nil
}

// CurrentMiniRound returns the mini-round that most recently finalized.
func (bs *blockchainState) CurrentMiniRound() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return uint64(bs.currentMiniRound), nil
}

// CurrentEpoch returns the epoch of the last finalized mini-round.
func (bs *blockchainState) CurrentEpoch() (uint64, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	return bs.currentEpoch, nil
}
