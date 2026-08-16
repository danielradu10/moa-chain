package blockFinalizer

import (
	"errors"
	"sync"

	"moa-chain/data"
)

// ErrNilFinalizedBlock signals that a nil block was provided for finalization.
var ErrNilFinalizedBlock = errors.New("nil finalized block")

// ErrFinalizedBlockNotFound signals that no finalized block exists for the requested round key.
var ErrFinalizedBlockNotFound = errors.New("finalized block not found")

// BlockFinalizer defines what should be done when a block is finalized in different mini-rounds.
type BlockFinalizer interface {
	FinalizeBlockMROne(roundKey data.RoundKey, block *data.BlockOnChain) error
	FinalizeBlockMRTwo(roundKey data.RoundKey, block *data.BlockOnChain) error
	FinalizeBlockMRThree(roundKey data.RoundKey, block *data.BlockOnChain) error
	GetFinalizedBlockInMROne(key data.RoundKey) (*data.BlockOnChain, error)
	GetFinalizedBlockInMRTwo(key data.RoundKey) (*data.BlockOnChain, error)
	GetFinalizedBlockInMRThree(key data.RoundKey) (*data.BlockOnChain, error)
	// ClearRound removes all three mini-round entries for the given full round.
	// The roundKey may be any of the three mini-round keys; the base round
	// (Epoch + Round) is used to derive and delete all three.
	ClearRound(roundKey data.RoundKey)
}

// FinalizeBlockComponent stores finalized blocks per round key.
type FinalizeBlockComponent struct {
	mutex sync.RWMutex

	finalizedBlocksInMROne   map[data.RoundKey]*data.BlockOnChain
	finalizedBlocksInMRTwo   map[data.RoundKey]*data.BlockOnChain
	finalizedBlocksInMRThree map[data.RoundKey]*data.BlockOnChain
}

// NewFinalizeBlockComponent creates a block finalizer component with in-memory state.
func NewFinalizeBlockComponent() *FinalizeBlockComponent {
	return &FinalizeBlockComponent{
		finalizedBlocksInMROne:   make(map[data.RoundKey]*data.BlockOnChain),
		finalizedBlocksInMRTwo:   make(map[data.RoundKey]*data.BlockOnChain),
		finalizedBlocksInMRThree: make(map[data.RoundKey]*data.BlockOnChain),
	}
}

// FinalizeBlockMROne stores the block finalized in mini-round one for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMROne(roundKey data.RoundKey, block *data.BlockOnChain) error {
	return component.finalizeBlock(component.finalizedBlocksInMROne, roundKey, block)
}

// FinalizeBlockMRTwo stores the block finalized in mini-round two for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMRTwo(roundKey data.RoundKey, block *data.BlockOnChain) error {
	return component.finalizeBlock(component.finalizedBlocksInMRTwo, roundKey, block)
}

// GetFinalizedBlockInMROne returns the block finalized in mini-round one for the provided round key.
func (component *FinalizeBlockComponent) GetFinalizedBlockInMROne(roundKey data.RoundKey) (*data.BlockOnChain, error) {
	return component.getFinalizedBlock(component.finalizedBlocksInMROne, roundKey)
}

// GetFinalizedBlockInMRTwo returns the block finalized in mini-round two for the provided round key.
func (component *FinalizeBlockComponent) GetFinalizedBlockInMRTwo(roundKey data.RoundKey) (*data.BlockOnChain, error) {
	return component.getFinalizedBlock(component.finalizedBlocksInMRTwo, roundKey)
}

// FinalizeBlockMRThree stores the block finalized in mini-round three for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMRThree(roundKey data.RoundKey, block *data.BlockOnChain) error {
	return component.finalizeBlock(component.finalizedBlocksInMRThree, roundKey, block)
}

// GetFinalizedBlockInMRThree returns the block finalized in mini-round three for the provided round key.
func (component *FinalizeBlockComponent) GetFinalizedBlockInMRThree(roundKey data.RoundKey) (*data.BlockOnChain, error) {
	return component.getFinalizedBlock(component.finalizedBlocksInMRThree, roundKey)
}

// ClearRound removes all three mini-round finalized-block entries for a
// completed full round (identified by Epoch + Round in the provided key).
func (component *FinalizeBlockComponent) ClearRound(roundKey data.RoundKey) {
	component.mutex.Lock()
	defer component.mutex.Unlock()

	for _, mr := range []data.MiniRound{data.MiniRoundOne, data.MiniRoundTwo, data.MiniRoundThree} {
		key := data.RoundKey{Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: uint64(mr)}
		delete(component.finalizedBlocksInMROne, key)
		delete(component.finalizedBlocksInMRTwo, key)
		delete(component.finalizedBlocksInMRThree, key)
	}
}

func (component *FinalizeBlockComponent) finalizeBlock(
	finalizedBlocks map[data.RoundKey]*data.BlockOnChain,
	roundKey data.RoundKey,
	block *data.BlockOnChain,
) error {
	if block == nil {
		return ErrNilFinalizedBlock
	}

	component.mutex.Lock()
	defer component.mutex.Unlock()

	finalizedBlocks[roundKey] = block
	return nil
}

func (component *FinalizeBlockComponent) getFinalizedBlock(
	finalizedBlocks map[data.RoundKey]*data.BlockOnChain,
	roundKey data.RoundKey,
) (*data.BlockOnChain, error) {
	component.mutex.RLock()
	defer component.mutex.RUnlock()

	block, ok := finalizedBlocks[roundKey]
	if !ok {
		return nil, ErrFinalizedBlockNotFound
	}

	return block, nil
}
