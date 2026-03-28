package mempool

import (
	"sync"
)

type memPool struct {
	transactionsByHash map[string]Transaction
	senders            *sendersMap

	mempoolMutex sync.RWMutex
}

func NewMemPool() *memPool {
	return &memPool{
		transactionsByHash: make(map[string]Transaction),
		senders:            newSendersMap(),
	}
}

func (mp *memPool) AddTransaction(transaction Transaction) error {
	if transaction == nil {
		return ErrNilTransaction
	}

	mp.mempoolMutex.Lock()
	defer mp.mempoolMutex.Unlock()

	mp.addTxByHashNoLock(transaction)

	sender := transaction.GetSender()
	mp.senders.add(string(sender), transaction)

	return nil
}

// addTxByHashNoLock saves a transaction by its hash; it has to be called under a mutex protection
func (mp *memPool) addTxByHashNoLock(transaction Transaction) {
	txHash := transaction.GetTxHash()
	_, ok := mp.transactionsByHash[string(txHash)]
	if !ok {
		mp.transactionsByHash[string(txHash)] = transaction
	}
}
