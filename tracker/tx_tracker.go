package tracker

import (
	"sync"

	"moa-chain/data"
)

// TxStatus represents a transaction's position in the processing lifecycle.
type TxStatus string

const (
	TxStatusSubmitted     TxStatus = "SUBMITTED"
	TxStatusPreprocessing TxStatus = "PREPROCESSING"
	TxStatusPending       TxStatus = "PENDING"
	TxStatusFinalized     TxStatus = "FINALIZED"
)

// TxTracker records the lifecycle status of every transaction the node has seen.
// It is updated via callback hooks injected into TxInterceptor and TxPreprocessor;
// no pipeline package imports tracker.
// All methods are safe for concurrent use.
type TxTracker struct {
	mu      sync.RWMutex
	entries map[string]TxStatus
	// OnStatusChanged is called after every status transition, outside the
	// internal lock. Used by the explorer to push events to SSE subscribers.
	// Optional; nil disables callbacks.
	OnStatusChanged func(txHash string, status TxStatus)
}

func NewTxTracker() *TxTracker {
	return &TxTracker{entries: make(map[string]TxStatus)}
}

// OnSubmitted is wired as TxInterceptorArgs.OnTxSubmitted.
// Called when a transaction passes validation and deduplication.
func (t *TxTracker) OnSubmitted(tx data.Transaction) {
	t.set(string(tx.GetTxHash()), TxStatusSubmitted)
}

// OnPreprocessing is wired as TxPreprocessorArgs.OnTxPreprocessing.
// Called when a transaction enters the preprocessor queue.
func (t *TxTracker) OnPreprocessing(tx data.Transaction) {
	t.set(string(tx.GetTxHash()), TxStatusPreprocessing)
}

// OnPending is wired as TxPreprocessorArgs.OnTxPending.
// Called after labels + answer are stored and the tx is added to the mempool.
func (t *TxTracker) OnPending(tx data.Transaction) {
	t.set(string(tx.GetTxHash()), TxStatusPending)
}

// OnFinalized is called when a tx appears in a canonical chain block.
func (t *TxTracker) OnFinalized(txHash string) {
	t.set(txHash, TxStatusFinalized)
}

// GetStatus returns the current tracked status for txHash.
// Returns false if the hash is unknown.
func (t *TxTracker) GetStatus(txHash string) (TxStatus, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.entries[txHash]
	return s, ok
}

func (t *TxTracker) set(txHash string, status TxStatus) {
	t.mu.Lock()
	t.entries[txHash] = status
	t.mu.Unlock()
	if t.OnStatusChanged != nil {
		t.OnStatusChanged(txHash, status)
	}
}
