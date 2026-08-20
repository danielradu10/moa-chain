package mempool

import (
	"moa-chain/data"
	"moa-chain/state"
)

type Mempool interface {
	AddTransaction(transaction data.Transaction) error
	SelectTransactions(accountsState state.AccountsState) []data.Transaction
	RemoveTransactions(txHashes [][]byte)
}
