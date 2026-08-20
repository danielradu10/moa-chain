package txpipeline

import "sync"

type precomputedStore struct {
	mu      sync.RWMutex
	labels  map[string][]string
	answers map[string]string
}

// NewPrecomputedStore creates a new empty PrecomputedStore.
func NewPrecomputedStore() PrecomputedStore {
	return &precomputedStore{
		labels:  make(map[string][]string),
		answers: make(map[string]string),
	}
}

func (s *precomputedStore) StoreLabels(txHash []byte, labels []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.labels[string(txHash)] = labels
}

func (s *precomputedStore) GetLabels(txHash []byte) ([]string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	labels, ok := s.labels[string(txHash)]
	return labels, ok
}

func (s *precomputedStore) StoreAnswer(txHash []byte, answer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.answers[string(txHash)] = answer
}

func (s *precomputedStore) GetAnswer(txHash []byte) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	answer, ok := s.answers[string(txHash)]
	return answer, ok
}

func (s *precomputedStore) Remove(txHash []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.labels, string(txHash))
	delete(s.answers, string(txHash))
}
