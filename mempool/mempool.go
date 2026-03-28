package mempool

import (
	"sync"
)

type memPool struct {
	transactionsCount  uint64
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

// AddTransaction adds a transaction into the mempool.
// Saves the transactions by its txHash and adds it to a map of senders.
// Each sender has a sorted transactions list.
func (mp *memPool) AddTransaction(transaction Transaction) error {
	mp.mempoolMutex.Lock()
	defer mp.mempoolMutex.Unlock()

	if transaction == nil {
		return ErrNilTransaction
	}

	// check if the transaction is not already added (case of duplicates)
	if mp.hasTransactionNoLock(transaction) {
		return nil
	}

	mp.addTxByHashNoLock(transaction)

	sender := transaction.GetSender()
	mp.senders.add(string(sender), transaction)

	mp.transactionsCount++
	return nil
}

func (mp *memPool) hasTransactionNoLock(transaction Transaction) bool {
	txHash := transaction.GetTxHash()
	_, ok := mp.transactionsByHash[string(txHash)]
	return ok
}

// addTxByHashNoLock saves a transaction by its hash; it has to be called under a mutex protection
func (mp *memPool) addTxByHashNoLock(transaction Transaction) {
	txHash := transaction.GetTxHash()
	_, ok := mp.transactionsByHash[string(txHash)]
	if !ok {
		mp.transactionsByHash[string(txHash)] = transaction
	}
}

// NumTransactions returns the number of transactions from the pool
func (mp *memPool) NumTransactions() uint64 {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	return uint64(len(mp.transactionsByHash))
}

// NumAddresses returns the number of addresses from the pool
func (mp *memPool) NumAddresses() uint64 {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	return mp.senders.numAddresses()
}

func (mp *memPool) getTransactionsListBySender(sender []byte) (*txList, error) {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	if sender == nil {
		return nil, ErrNilSender
	}

	return mp.senders.getTransactionsListBySender(sender)
}
