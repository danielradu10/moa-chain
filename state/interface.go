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
	LoadEscrowAccount() (AccountHandler, error)
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
	CurrentRound() (uint64, error)
	CurrentMiniRound() (uint64, error)
	CurrentEpoch() (uint64, error)
}

// AccountsSnapshot defines what an AccountsSnapshot should do
type AccountsSnapshot interface {
	AccountsProvider
	Commit() error
	Discard()
}

// AccountsSnapshotFactory defines what a AccountsSnapshotFactory should do
type AccountsSnapshotFactory interface {
	CreateSnapshot() (AccountsSnapshot, error)
}

// RoundState defines what a round state should do.
type RoundState interface {
	SetProposedBlock(roundKey data.RoundKey, block *data.Block) error
	GetProposedBlock(roundKey data.RoundKey) (*data.Block, error)

	SetCertificate(roundKey data.RoundKey, certificate *data.AggregatedVotes) error
	IsCertificateSet(roundKey data.RoundKey) bool

	AddVote(roundKey data.RoundKey, vote *data.BlockVote) error
	GetVotes(roundKey data.RoundKey) ([]*data.ValidatorVote, error)

	AddExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error
	GetExecutedPromptsMessages(roundKey data.RoundKey) ([]*data.AnswersBlockMessage, error)

	AddAnswerClassificationVote(roundKey data.RoundKey, vote *data.AnswerClassificationVote) error
	GetAnswerClassificationVotes(roundKey data.RoundKey) ([]*data.AnswerClassificationVote, error)
	SetAnswerClassificationCertificate(roundKey data.RoundKey, certificate *data.AnswerClassificationCertificate) error
	GetAnswerClassificationCertificate(roundKey data.RoundKey) (*data.AnswerClassificationCertificate, error)

	ClearRoundState(roundKey data.RoundKey)
}
