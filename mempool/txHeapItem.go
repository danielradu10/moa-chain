package mempool

type txHeapItem struct {
	senderTxList *txList

	currentIndex             uint64
	currentTransactionInHeap Transaction
}

func newTxHeapItem(senderTxList *txList) *txHeapItem {
	return &txHeapItem{
		senderTxList:             senderTxList,
		currentIndex:             0,
		currentTransactionInHeap: senderTxList.getTxByIndex(0),
	}
}

func (item *txHeapItem) getCurrentTransaction() Transaction {
	return item.currentTransactionInHeap
}

func (item *txHeapItem) nextTransactionOfSenderExists() bool {
	return int(item.currentIndex+1) < item.senderTxList.numTransactions()
}

func (item *txHeapItem) goToNextTransactionOfSender() {
	item.currentIndex++
	item.currentTransactionInHeap = item.senderTxList.getTxByIndex(item.currentIndex)
}
