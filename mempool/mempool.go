package mempool

import (
	"container/heap"
	"sync"
)

const maxBlockConsumption = 10000

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

// snapshot does a snapshot of each important map from the MemPool.
// The method is used in the SelectTransactions so that the selection can be made without concurrent interferences.
func (mp *memPool) snapshot() (map[string]Transaction, *sendersMap) {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	transactionsByHashSnapshot := make(map[string]Transaction, len(mp.transactionsByHash))
	for txHash, tx := range mp.transactionsByHash {
		transactionsByHashSnapshot[txHash] = tx
	}

	sendersMapSnapshot := mp.senders.snapshot()
	return transactionsByHashSnapshot, sendersMapSnapshot
}

func (mp *memPool) SelectTransactions() []Transaction {
	_, sendersMapSnapshot := mp.snapshot()

	txHeap, err := newTransactionsHeap(sendersMapSnapshot.numAddresses(), nil)
	if err != nil {
		return nil
	}

	heap.Init(txHeap)
	for _, senderTxList := range sendersMapSnapshot.senders {
		heap.Push(txHeap, txHeapItem{senderTxList: senderTxList})
	}

	accumulatedConsumption := uint64(0)
	selectedTransactions := make([]Transaction, 0)
	selSession := newSelectionSession()

	for txHeap.Len() > 0 {
		currentBestItem := heap.Pop(txHeap).(txHeapItem)

		currentBestTransaction := currentBestItem.getCurrentTransaction()
		estimatedConsumption := currentBestTransaction.GetEstimatedConsumption()
		if accumulatedConsumption+estimatedConsumption > maxBlockConsumption {
			break
		}

		if selSession.senderShouldBeSkipped(currentBestTransaction) {
			continue
		}

		if !selSession.transactionShouldBeSkipped(currentBestTransaction) {
			err := selSession.OnSelectedTransaction(currentBestTransaction)
			if err != nil {
				continue
			}

			selectedTransactions = append(selectedTransactions, currentBestTransaction)
			accumulatedConsumption += estimatedConsumption
		}

		if currentBestItem.nextTransactionOfSenderExists() {
			currentBestItem.goToNextTransactionOfSender()
			heap.Push(txHeap, currentBestItem)
		}
	}

	return selectedTransactions
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
