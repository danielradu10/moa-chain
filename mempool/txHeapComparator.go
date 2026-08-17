package mempool

import (
	"bytes"
)

type txHeapComparator func(transactionA *txHeapItem, transactionB *txHeapItem) bool

// isTransactionMoreValuable orders candidates by:
//  1. estimated score descending (tip / estimatedConsumption)
//  2. estimated consumption ascending
//  3. sender ascending
//  4. nonce ascending
//
// The final two tiebreakers match the nonce ordering that the mempool enforces
// within a single sender's transaction sequence, keeping selection and block
// validation consistent.
func isTransactionMoreValuable(transactionA *txHeapItem, transactionB *txHeapItem) bool {
	txA := transactionA.getCurrentTransaction()
	txB := transactionB.getCurrentTransaction()
	if txA.GetEstimatedScore() != txB.GetEstimatedScore() {
		return txA.GetEstimatedScore() > txB.GetEstimatedScore()
	}

	if txA.GetEstimatedConsumption() != txB.GetEstimatedConsumption() {
		return txA.GetEstimatedConsumption() < txB.GetEstimatedConsumption()
	}

	senderCmp := bytes.Compare(txA.GetSender(), txB.GetSender())
	if senderCmp != 0 {
		return senderCmp < 0
	}

	return txA.GetNonce() < txB.GetNonce()
}
