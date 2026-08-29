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

// BlockFinalizerHooks holds optional observer hooks called after each mini-round finalization.
// Wire tracker.RoundTracker methods here at setup time. Nil fields are ignored.
type BlockFinalizerHooks struct {
	OnMR1Finalized func(data.RoundKey, *data.BlockOnChain)
	OnMR2Finalized func(data.RoundKey, *data.BlockOnChain)
	OnMR3Finalized func(data.RoundKey, *data.BlockOnChain)
}

// FinalizeBlockComponent stores finalized blocks per round key.
type FinalizeBlockComponent struct {
	mutex sync.RWMutex
	hooks BlockFinalizerHooks

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

// WithHooks sets optional observer hooks and returns the receiver for chaining.
// Must be called once at wiring time before any mini-rounds begin.
func (component *FinalizeBlockComponent) WithHooks(hooks BlockFinalizerHooks) *FinalizeBlockComponent {
	component.hooks = hooks
	return component
}

// FinalizeBlockMROne stores the block finalized in mini-round one for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMROne(roundKey data.RoundKey, block *data.BlockOnChain) error {
	if err := component.finalizeBlock(component.finalizedBlocksInMROne, roundKey, block); err != nil {
		return err
	}

	if component.hooks.OnMR1Finalized != nil {
		component.hooks.OnMR1Finalized(roundKey, block)
	}

	return nil
}

// FinalizeBlockMRTwo stores the block finalized in mini-round two for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMRTwo(roundKey data.RoundKey, block *data.BlockOnChain) error {
	if err := component.finalizeBlock(component.finalizedBlocksInMRTwo, roundKey, block); err != nil {
		return err
	}

	if component.hooks.OnMR2Finalized != nil {
		component.hooks.OnMR2Finalized(roundKey, block)
	}

	return nil
}

// FinalizeBlockMRThree stores the block finalized in mini-round three for the provided round key.
func (component *FinalizeBlockComponent) FinalizeBlockMRThree(roundKey data.RoundKey, block *data.BlockOnChain) error {
	if err := component.finalizeBlock(component.finalizedBlocksInMRThree, roundKey, block); err != nil {
		return err
	}

	if component.hooks.OnMR3Finalized != nil {
		component.hooks.OnMR3Finalized(roundKey, block)
	}

	return nil
}

// GetFinalizedBlockInMROne returns the block finalized in mini-round one for the provided round key.
func (component *FinalizeBlockComponent) GetFinalizedBlockInMROne(roundKey data.RoundKey) (*data.BlockOnChain, error) {
	return component.getFinalizedBlock(component.finalizedBlocksInMROne, roundKey)
}

// GetFinalizedBlockInMRTwo returns the block finalized in mini-round two for the provided round key.
func (component *FinalizeBlockComponent) GetFinalizedBlockInMRTwo(roundKey data.RoundKey) (*data.BlockOnChain, error) {
	return component.getFinalizedBlock(component.finalizedBlocksInMRTwo, roundKey)
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
