package broadcast

import (
	"sync"

	"moa-chain/data"
)

type txPeerRegistry struct {
	mu       sync.RWMutex
	registry map[string]chan<- data.Transaction
}

// NewTxPeerRegistry creates an empty TxPeerRegistry.
func NewTxPeerRegistry() TxPeerRegistry {
	return &txPeerRegistry{
		registry: make(map[string]chan<- data.Transaction),
	}
}

func (r *txPeerRegistry) Register(nodeID string, inbox chan<- data.Transaction) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.registry[nodeID]; exists {
		return ErrValidatorAlreadyExists
	}
	r.registry[nodeID] = inbox
	return nil
}

func (r *txPeerRegistry) GetAll() map[string]chan<- data.Transaction {
	r.mu.RLock()
	defer r.mu.RUnlock()

	snapshot := make(map[string]chan<- data.Transaction, len(r.registry))
	for id, ch := range r.registry {
		snapshot[id] = ch
	}
	return snapshot
}
