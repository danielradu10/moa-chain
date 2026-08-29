package tracker

import (
	"sync"

	"moa-chain/data"
)

// RoundStatus represents how far a round has progressed through its mini-rounds.
type RoundStatus string

const (
	RoundStatusMR1Complete RoundStatus = "MR1_COMPLETE"
	RoundStatusMR2Complete RoundStatus = "MR2_COMPLETE"
	RoundStatusMR3Complete RoundStatus = "MR3_COMPLETE"
)

// RoundEntry holds the accumulated state for one full round.
// Blocks are set as each mini-round finalizes; absent mini-rounds have nil blocks.
type RoundEntry struct {
	Epoch    uint64
	Round    uint64
	Status   RoundStatus
	MR1Block *data.BlockOnChain
	MR2Block *data.BlockOnChain
	MR3Block *data.BlockOnChain
}

type roundKey struct {
	epoch uint64
	round uint64
}

// RoundTracker records the finalization progress of every round the node has seen.
// It is updated via callback hooks injected into blockFinalizer; no blockFinalizer
// package imports tracker.
// All methods are safe for concurrent use.
type RoundTracker struct {
	mu      sync.RWMutex
	entries map[roundKey]*RoundEntry
}

func NewRoundTracker() *RoundTracker {
	return &RoundTracker{entries: make(map[roundKey]*RoundEntry)}
}

// OnMR1Finalized is wired as blockFinalizer.BlockFinalizerHooks.OnMR1Finalized.
func (t *RoundTracker) OnMR1Finalized(key data.RoundKey, block *data.BlockOnChain) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(key)
	e.MR1Block = block
	e.Status = RoundStatusMR1Complete
}

// OnMR2Finalized is wired as blockFinalizer.BlockFinalizerHooks.OnMR2Finalized.
func (t *RoundTracker) OnMR2Finalized(key data.RoundKey, block *data.BlockOnChain) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(key)
	e.MR2Block = block
	e.Status = RoundStatusMR2Complete
}

// OnMR3Finalized is wired as blockFinalizer.BlockFinalizerHooks.OnMR3Finalized.
func (t *RoundTracker) OnMR3Finalized(key data.RoundKey, block *data.BlockOnChain) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.getOrCreate(key)
	e.MR3Block = block
	e.Status = RoundStatusMR3Complete
}

// GetRound returns a snapshot of the entry for the given epoch+round pair.
// Returns false if the round has not been seen yet.
func (t *RoundTracker) GetRound(epoch, round uint64) (RoundEntry, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	e, ok := t.entries[roundKey{epoch: epoch, round: round}]
	if !ok {
		return RoundEntry{}, false
	}
	return *e, true
}

func (t *RoundTracker) getOrCreate(key data.RoundKey) *RoundEntry {
	rk := roundKey{epoch: key.Epoch, round: key.Round}
	if e, ok := t.entries[rk]; ok {
		return e
	}
	e := &RoundEntry{Epoch: key.Epoch, Round: key.Round}
	t.entries[rk] = e
	return e
}
