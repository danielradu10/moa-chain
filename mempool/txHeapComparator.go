package mempool

import (
	"bytes"
)

type txHeapComparator func(transactionA *txHeapItem, transactionB *txHeapItem) bool

// isTransactionMoreValuable first sorts by the estimated score (tip / estimatedConsumption)
// then, it sorts by estimated consumption
// lastly, it sorts by txHash
func isTransactionMoreValuable(transactionA *txHeapItem, transactionB *txHeapItem) bool {
	txA := transactionA.getCurrentTransaction()
	txB := transactionB.getCurrentTransaction()
	if txA.GetEstimatedScore() != txB.GetEstimatedScore() {
		return txA.GetEstimatedScore() > txB.GetEstimatedScore()
	}

	if txA.GetEstimatedConsumption() != txB.GetEstimatedConsumption() {
		return txA.GetEstimatedConsumption() < txB.GetEstimatedConsumption()
	}

	return bytes.Compare(txA.GetTxHash(), txB.GetTxHash()) < 0
}
