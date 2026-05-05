package validators

import (
	"moa-chain/data"
	"moa-chain/state"
)

// ValidatorRegistry defines what a validator registry shoul do.
type ValidatorRegistry interface {
	GetPublicKey(validatorID string) ([]byte, error)

	IsValidatorRegistered(validatorID string) bool
	IsValidatorInConsensusGroup(validatorID string) bool

	IsNodeLeader(validatorID string) bool
	LeaderOfConsensusGroup() (string, error)

	GenerateConsensusGroup(blockchainState state.BlockchainState, roundKey data.RoundKey) error
	ConsensusGroup() ([]string, error)
	ConsensusGroupSize() (uint64, error)

	GetValidatorsIDs() []string
}

// ConsensusSelector defines what a consensus selector should do.
type ConsensusSelector interface {
	SelectConsensusGroup(blockchainState state.BlockchainState, validators []*Validator, roundKey data.RoundKey) ([]string, error)
	Leader() (string, error)
	ConsensusGroup() ([]string, error)
	ConsensusGroupSize() (uint64, error)
}
