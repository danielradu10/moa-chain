package mempool

import (
	"sync"
)

type sendersMap struct {
	senders       map[string]*txList
	mutSendersMap sync.RWMutex
}

func newSendersMap() *sendersMap {
	return &sendersMap{
		senders: make(map[string]*txList),
	}
}

// add adds a new sender into the senders map and appends its transaction in its sorted list
func (sm *sendersMap) add(sender string, tx Transaction) {
	sm.mutSendersMap.Lock()
	defer sm.mutSendersMap.Unlock()

	_, ok := sm.senders[sender]
	if !ok {
		sm.senders[sender] = newTxList()
	}

	sm.senders[sender].add(tx)
}
