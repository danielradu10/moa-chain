package mempool

import (
	"bytes"
	"sync"

	"moa-chain/data"
)

type txList struct {
	mutTxList       sync.RWMutex
	transactionList []data.Transaction
}

func newTxList() *txList {
	return &txList{
		transactionList: make([]data.Transaction, 0),
	}
}

// add adds a new transaction and keeps the transactions sorted
func (txl *txList) add(tx data.Transaction) {
	txl.mutTxList.Lock()
	defer txl.mutTxList.Unlock()

	insertionPlace := txl.findInsertionPlaceNoLock(tx)

	updatedList := make([]data.Transaction, 0, len(txl.transactionList)+1)
	updatedList = append(updatedList, txl.transactionList[:insertionPlace]...)
	updatedList = append(updatedList, tx)
	updatedList = append(updatedList, txl.transactionList[insertionPlace:]...)

	txl.transactionList = updatedList
}

// findInsertionPlaceNoLock finds the insertion place of a transaction.
// The method has to be called under a mutex protection
func (txl *txList) findInsertionPlaceNoLock(tx data.Transaction) uint64 {
	left := 0
	right := len(txl.transactionList)

	for left < right {
		mid := left + (right-left)/2

		if shouldComeBefore(txl.transactionList[mid], tx) {
			left = mid + 1
		} else {
			right = mid
		}
	}

	return uint64(left)
}

// shouldComeBefore returns true if transactionA should be placed before transactionB
func shouldComeBefore(transactionA data.Transaction, transactionB data.Transaction) bool {
	// the most important criteria: nonce
	if transactionA.GetNonce() != transactionB.GetNonce() {
		return transactionA.GetNonce() < transactionB.GetNonce()
	}

	// in case two transaction have the same nonce, sort them in ascending order by estimated consumption
	if transactionA.GetEstimatedConsumption() != transactionB.GetEstimatedConsumption() {
		return transactionA.GetEstimatedConsumption() < transactionB.GetEstimatedConsumption()
	}

	// we need a deterministic way in case of equal transactions
	return bytes.Compare(transactionA.GetTxHash(), transactionB.GetTxHash()) < 0
}

// numTransactions returns the number of transactions in the list
func (txl *txList) numTransactions() int {
	txl.mutTxList.RLock()
	defer txl.mutTxList.RUnlock()

	return len(txl.transactionList)
}

func (txl *txList) getTxByIndex(index uint64) data.Transaction {
	txl.mutTxList.RLock()
	defer txl.mutTxList.RUnlock()

	if int(index) >= len(txl.transactionList) {
		return nil
	}

	return txl.transactionList[index]
}

// remove removes the transaction with the given hash from the list.
// The order of remaining transactions is preserved.
func (txl *txList) remove(txHash []byte) {
	txl.mutTxList.Lock()
	defer txl.mutTxList.Unlock()

	for i, tx := range txl.transactionList {
		if bytes.Equal(tx.GetTxHash(), txHash) {
			txl.transactionList = append(txl.transactionList[:i], txl.transactionList[i+1:]...)
			return
		}
	}
}

func (txl *txList) snapshot() *txList {
	txl.mutTxList.RLock()
	defer txl.mutTxList.RUnlock()

	transactionListCopy := make([]data.Transaction, len(txl.transactionList))
	copy(transactionListCopy, txl.transactionList)

	return &txList{
		transactionList: transactionListCopy,
	}
}
