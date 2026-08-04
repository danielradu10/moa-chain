package transactionprocessing

import (
	"moa-chain/data"
)

// TxProcessor defines what a TxProcessor should do
type TxProcessor interface {
	ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	ValidateTransactionsOrdering(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	SelectTransactions() []data.Transaction
}
