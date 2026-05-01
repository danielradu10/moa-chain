package consensus

import (
	"moa-chain/data"
)

// ValidatorRegistry defines what a validator registry shoul do.
type ValidatorRegistry interface {
	GetPublicKey(validatorID string) ([]byte, error)

	IsValidatorRegistered(validatorID string) bool
	IsValidatorInConsensusGroup(validatorID string) bool
	IsNodeLeader(validatorID string) bool

	GenerateConsensusGroup(roundKey data.RoundKey) error
	ConsensusGroup() [][]byte
	ConsensusGroupSize() uint64
}

// RoundState defines what a round state should do.
type RoundState interface {
	SetProposedBlock(roundKey data.RoundKey, block *data.Block) error
	GetProposedBlock(roundKey data.RoundKey) (*data.Block, error)

	AddVote(roundKey data.RoundKey, vote *data.BlockVote) error
	GetVotes(roundKey data.RoundKey) ([]*data.ValidatorVote, error)

	ClearRoundState(roundKey data.RoundKey)
}

// MiniRoundOneHandler defines what a MiniRound One Handler should do
type MiniRoundOneHandler interface {
	HandleProposingBlock(roundKey data.RoundKey) error
	HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error
	HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error
	HandleAggregatedVotes(roundKey data.RoundKey, votes *data.AggregatedVotes) error
}
