package testscommon

import (
	"moa-chain/data"
)

type TxProcessorStub struct {
	ProcessTransactionCalled           func(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	ValidateTransactionsOrderingCalled func(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	Called                             bool
}

// ProcessTransaction -
func (tps *TxProcessorStub) ProcessTransaction(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
	tps.Called = true

	if tps.ProcessTransactionCalled != nil {
		return tps.ProcessTransactionCalled(tx, miniRound)
	}

	return 0, nil
}

// ValidateTransactionsOrdering -
func (tps *TxProcessorStub) ValidateTransactionsOrdering(previousTransaction data.Transaction, currentTransaction data.Transaction) error {
	if tps.ValidateTransactionsOrderingCalled != nil {
		return tps.ValidateTransactionsOrderingCalled(previousTransaction, currentTransaction)
	}

	return nil
}
