package testscommon

import (
	"moa-chain/data"
)

// TxProcessorStub -
type TxProcessorStub struct {
	ProcessTransactionCalled           func(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	ValidateTransactionsOrderingCalled func(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	LabelTransactionCalled             func(tx data.Transaction, amILeader bool) ([]string, error)
	ValidateLabelsCalled               func(labelsGeneratedByLeader []string, labelsGeneratedByMe []string) error
	SelectTransactionsCalled           func() []data.Transaction
	Called                             bool
}

// ProcessTransactionEconomically -
func (tps *TxProcessorStub) ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
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

// LabelTransaction -
func (tps *TxProcessorStub) LabelTransaction(tx data.Transaction, amILeader bool) ([]string, error) {
	if tps.LabelTransactionCalled != nil {
		return tps.LabelTransactionCalled(tx, amILeader)
	}

	return nil, nil
}

// ValidateLabels -
func (tps *TxProcessorStub) ValidateLabels(labelsGeneratedByLeader []string, labelsGeneratedByMe []string) error {
	if tps.ValidateLabelsCalled != nil {
		return tps.ValidateLabelsCalled(labelsGeneratedByLeader, labelsGeneratedByMe)
	}

	return nil
}

// SelectTransactions -
func (tps *TxProcessorStub) SelectTransactions() []data.Transaction {
	if tps.SelectTransactionsCalled != nil {
		return tps.SelectTransactionsCalled()
	}

	return nil
}
