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

func (sm *sendersMap) numAddresses() uint64 {
	sm.mutSendersMap.RLock()
	defer sm.mutSendersMap.RUnlock()

	return uint64(len(sm.senders))
}

func (sm *sendersMap) getTransactionsListBySender(sender []byte) (*txList, error) {
	sm.mutSendersMap.RLock()
	defer sm.mutSendersMap.RUnlock()

	if sender == nil {
		return nil, ErrNilSender
	}

	txs, ok := sm.senders[string(sender)]
	if !ok {
		return nil, ErrSenderDoesNotExist
	}

	return txs, nil
}
