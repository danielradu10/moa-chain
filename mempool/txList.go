package mempool

import (
	"bytes"
	"sync"
)

type txList struct {
	mutTxList       sync.Mutex
	transactionList []Transaction
}

func newTxList() *txList {
	return &txList{
		transactionList: make([]Transaction, 0),
	}
}

// add adds a new transaction and keeps the transactions sorted
func (txl *txList) add(tx Transaction) {
	txl.mutTxList.Lock()
	defer txl.mutTxList.Unlock()

	insertionPlace := txl.findInsertionPlaceNoLock(tx)

	updatedList := make([]Transaction, 0, len(txl.transactionList)+1)
	updatedList = append(updatedList, txl.transactionList[:insertionPlace]...)
	updatedList = append(updatedList, tx)
	updatedList = append(updatedList, txl.transactionList[insertionPlace:]...)

	txl.transactionList = updatedList
}

// findInsertionPlaceNoLock finds the insertion place of a transaction; the method has to be called under a mutex lock
func (txl *txList) findInsertionPlaceNoLock(tx Transaction) uint64 {
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
func shouldComeBefore(transactionA Transaction, transactionB Transaction) bool {
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
