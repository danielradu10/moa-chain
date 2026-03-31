package state

type AccountsState interface {
	GetNonceByAddress(address string) (uint64, error)
	GetBalanceByAddress(address string) (uint64, error)
}
