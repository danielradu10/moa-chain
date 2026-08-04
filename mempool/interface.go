package mempool

import (
	"moa-chain/data"
	"moa-chain/state"
)

type Mempool interface {
	SelectTransactions(accountsState state.AccountsState) []data.Transaction
}
