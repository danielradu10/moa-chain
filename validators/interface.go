package validators

import (
	"moa-chain/data"
	"moa-chain/state"
)

// ValidatorRegistry defines what a validator registry shoul do.
type ValidatorRegistry interface {
	Register(validatorID string, validator *Validator) error

	GetPublicKey(validatorID string) ([]byte, error)

	IsValidatorRegistered(validatorID string) bool
	IsValidatorInConsensusGroup(validatorID string) bool

	IsNodeLeader(validatorID string) bool
	LeaderOfConsensusGroup() (string, error)

	GenerateConsensusGroupMiniRoundOne(blockchainState state.BlockchainState, roundKey data.RoundKey) error
	GenerateConsensusGroupMiniRoundTwo(blockchainState state.BlockchainState, roundKey data.RoundKey, frequencyMap map[string]uint64) error
	ConsensusGroup() ([]string, error)
	ConsensusGroupSize() (uint64, error)

	GetValidatorsIDs() []string
}

// ConsensusSelector defines what a consensus selector should do.
type ConsensusSelector interface {
	SelectConsensusGroupMiniRoundOne(blockchainState state.BlockchainState, validators []*Validator, roundKey data.RoundKey) ([]string, error)
	SelectConsensusGroupMiniRoundTwo(blockchainState state.BlockchainState, validators []*Validator, roundKey data.RoundKey, frequencyMap map[string]uint64) ([]string, error)
	Leader() (string, error)
	ConsensusGroup() ([]string, error)
	ConsensusGroupSize() (uint64, error)
}
