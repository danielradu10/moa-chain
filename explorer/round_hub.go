package explorer

import (
	"sync"

	"moa-chain/data"
)

// StepEvent is the payload pushed to SSE subscribers on each consensus step change.
type StepEvent struct {
	RoundKey data.RoundKey
	Step     data.Step
}

// RoundHub fans out step-change events from the consensus round handler to SSE
// subscribers. Wire OnStepChanged into RoundHandlerArgs.OnStepChanged at startup.
type RoundHub struct {
	mu   sync.Mutex
	subs map[uint64]chan StepEvent
	last *StepEvent
	next uint64
}

// NewRoundHub creates an empty hub with no subscribers.
func NewRoundHub() *RoundHub {
	return &RoundHub{subs: make(map[uint64]chan StepEvent)}
}

// Subscribe returns a buffered channel of StepEvents and an unsubscribe function.
// The caller must invoke the unsubscribe function when done to release the slot.
// If a state has already been published the channel immediately carries it, so
// the client sees the current step the moment it connects.
func (h *RoundHub) Subscribe() (<-chan StepEvent, func()) {
	h.mu.Lock()
	id := h.next
	h.next++
	ch := make(chan StepEvent, 16)
	h.subs[id] = ch
	if h.last != nil {
		ch <- *h.last
	}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		delete(h.subs, id)
		close(ch)
		h.mu.Unlock()
	}
}

// OnStepChanged is called on every consensus step transition. Wire this into
// RoundHandlerArgs.OnStepChanged. It fans the event out to all active
// subscribers non-blocking; slow subscribers are dropped rather than blocked.
func (h *RoundHub) OnStepChanged(key data.RoundKey, step data.Step) {
	e := StepEvent{RoundKey: key, Step: step}
	h.mu.Lock()
	h.last = &e
	for _, ch := range h.subs {
		select {
		case ch <- e:
		default:
		}
	}
	h.mu.Unlock()
}
