package explorer

import (
	"encoding/hex"
	"sync"

	"moa-chain/tracker"
)

// TxEvent is the payload pushed to SSE subscribers on each transaction status change.
type TxEvent struct {
	TxHash string           `json:"tx_hash"` // hex-encoded
	Status tracker.TxStatus `json:"status"`
}

// TxHub fans out transaction status-change events to per-hash SSE subscribers.
// Wire OnStatusChanged into TxTracker.OnStatusChanged at startup.
type TxHub struct {
	mu   sync.Mutex
	subs map[string]map[uint64]chan TxEvent // raw txHash → id → channel
	last map[string]TxEvent                // raw txHash → last known event
	next uint64
}

// NewTxHub creates an empty hub with no subscribers.
func NewTxHub() *TxHub {
	return &TxHub{
		subs: make(map[string]map[uint64]chan TxEvent),
		last: make(map[string]TxEvent),
	}
}

// Subscribe returns a buffered channel of TxEvents for the given raw-bytes
// transaction hash and an unsubscribe function. The caller must invoke
// unsubscribe when done to release the slot. If a status has already been
// published the channel immediately carries it. If the last known status is
// FINALIZED the channel is closed after delivering that event, so the handler
// returns without needing an explicit client disconnect.
func (h *TxHub) Subscribe(txHash string) (<-chan TxEvent, func()) {
	h.mu.Lock()

	id := h.next
	h.next++
	ch := make(chan TxEvent, 8)

	alreadyFinalized := false
	if last, ok := h.last[txHash]; ok {
		ch <- last
		if last.Status == tracker.TxStatusFinalized {
			alreadyFinalized = true
		}
	}

	if alreadyFinalized {
		close(ch)
	} else {
		if h.subs[txHash] == nil {
			h.subs[txHash] = make(map[uint64]chan TxEvent)
		}
		h.subs[txHash][id] = ch
	}

	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if subs, ok := h.subs[txHash]; ok {
			if _, exists := subs[id]; exists {
				delete(subs, id)
				close(ch)
			}
		}
		h.mu.Unlock()
	}
}

// OnStatusChanged is called on every transaction status transition. Wire this
// into TxTracker.OnStatusChanged. It fans the event to all active subscribers
// for the given hash non-blocking; slow subscribers are dropped. When status
// is FINALIZED the subscriber channels are closed so handlers return naturally.
func (h *TxHub) OnStatusChanged(txHash string, status tracker.TxStatus) {
	e := TxEvent{
		TxHash: hex.EncodeToString([]byte(txHash)),
		Status: status,
	}
	h.mu.Lock()
	h.last[txHash] = e
	for id, ch := range h.subs[txHash] {
		select {
		case ch <- e:
		default:
		}
		if status == tracker.TxStatusFinalized {
			close(ch)
			delete(h.subs[txHash], id)
		}
	}
	h.mu.Unlock()
}
