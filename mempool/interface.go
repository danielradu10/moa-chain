package mempool

import (
	"moa-chain/data"
	"moa-chain/state"
)

type Mempool interface {
	AddTransaction(transaction data.Transaction) error
	SelectTransactions(accountsState state.AccountsState) []data.Transaction
	RemoveTransactions(txHashes [][]byte)
	// GetPendingTransactions returns a snapshot of all transactions currently
	// in the pool, in no guaranteed order. Used by the explorer API.
	GetPendingTransactions() []data.Transaction
}
