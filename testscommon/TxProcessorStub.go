package testscommon

import (
	"moa-chain/data"
)

// TxProcessorStub -
type TxProcessorStub struct {
	ProcessTransactionCalled           func(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	ValidateTransactionsOrderingCalled func(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	LabelTransactionCalled             func(tx data.Transaction) ([]string, error)
	SelectTransactionsCalled           func() []data.Transaction
	ExecutePromptTransactionCalled     func(tx data.Transaction) (*data.TransactionResult, error)
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
func (tps *TxProcessorStub) LabelTransaction(tx data.Transaction) ([]string, error) {
	if tps.LabelTransactionCalled != nil {
		return tps.LabelTransactionCalled(tx)
	}

	return nil, nil
}

// SelectTransactions -
func (tps *TxProcessorStub) SelectTransactions() []data.Transaction {
	if tps.SelectTransactionsCalled != nil {
		return tps.SelectTransactionsCalled()
	}

	return nil
}

func (tps *TxProcessorStub) ExecutePromptTransaction(tx data.Transaction) (*data.TransactionResult, error) {
	if tps.ExecutePromptTransactionCalled != nil {
		return tps.ExecutePromptTransactionCalled(tx)
	}

	return &data.TransactionResult{
		TxHash: tx.GetTxHash(),
	}, nil
}
