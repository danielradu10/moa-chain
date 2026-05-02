package validators

import (
	"moa-chain/data"
)

// ValidatorRegistry defines what a validator registry shoul do.
type ValidatorRegistry interface {
	GetPublicKey(validatorID string) ([]byte, error)

	IsValidatorRegistered(validatorID string) bool
	IsValidatorInConsensusGroup(validatorID string) bool

	IsNodeLeader(validatorID string) bool
	LeaderOfConsensusGroup() ([]byte, error)

	GenerateConsensusGroup(roundKey data.RoundKey) error
	ConsensusGroup() [][]byte
	ConsensusGroupSize() uint64

	GetValidatorsIDs() []string
}
