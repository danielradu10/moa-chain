package transactionprocessing

import (
	"moa-chain/data"
)

// TxProcessor defines what a TxProcessor should do
type TxProcessor interface {
	ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error)
	LabelTransaction(tx data.Transaction, amILeader bool) ([]string, error)
	ValidateTransactionsOrdering(previousTransaction data.Transaction, currentTransaction data.Transaction) error
	ValidateLabels(labelsGeneratedByLeader []string, labelsGeneratedByMe []string) error
	SelectTransactions() []data.Transaction
}
