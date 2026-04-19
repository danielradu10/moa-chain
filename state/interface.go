package state

import (
	"moa-chain/data"
)

// AccountsState defines what an AccountsState should do
type AccountsState interface {
	GetNonceByAddress(address string) (uint64, error)
	GetBalanceByAddress(address string) (uint64, error)
}

// AccountsProvider defines what an accounts provider should do
type AccountsProvider interface {
	LoadAccount(address string) (AccountHandler, error)
}

// AccountHandler defines what an AccountHandler should do
type AccountHandler interface {
	Nonce() (uint64, error)
	Balance() (uint64, error)
	IncreaseNonce(increment uint64) error
	IncreaseBalance(increment uint64) error
	DecreaseBalance(decrement uint64) error
}

// BlockchainState defines what an BlockchainState component should do
type BlockchainState interface {
	CurrentBlockHeader() (*data.BlockHeader, error)
	CurrentBlock() (*data.Block, error)
	CurrentRound() (uint64, error)
	CurrentMiniRound() (uint64, error)
	CurrentEpoch() (uint64, error)
}
