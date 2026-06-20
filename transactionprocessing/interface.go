package transactionprocessing

import (
	"moa-chain/data"
)

// TxProcessor defines what a TxProcessor should do
type TxProcessor interface {
	ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	LabelTransaction(tx data.Transaction) ([]string, error)
	ValidateTransactionsOrdering(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	ExecutePromptTransaction(tx data.Transaction) (*data.TransactionResult, error)
	SelectTransactions() []data.Transaction
}
