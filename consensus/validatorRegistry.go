package consensus

import (
	"bytes"

	"moa-chain/data"
)

type validatorRegistry struct {
	currentLeader []byte

	validators     map[string][]byte
	consensusGroup map[string][]byte
}

// NewValidatorRegistry creates a new validator registry.
func NewValidatorRegistry() *validatorRegistry {
	return &validatorRegistry{
		validators:     make(map[string][]byte),
		consensusGroup: make(map[string][]byte),
	}
}

// GetPublicKey returns the public key of a validatorID.
func (vr *validatorRegistry) GetPublicKey(validatorID string) ([]byte, error) {
	_, ok := vr.validators[validatorID]
	if !ok {
		return nil, ErrInvalidValidator
	}

	return vr.validators[validatorID], nil
}

// IsValidatorRegistered returns if a validator is registered or not.
func (vr *validatorRegistry) IsValidatorRegistered(validatorID string) bool {
	_, ok := vr.validators[validatorID]
	return ok
}

// IsValidatorInConsensusGroup returns if a validator is in the consensus group or not.
func (vr *validatorRegistry) IsValidatorInConsensusGroup(validatorID string) bool {
	_, ok := vr.validators[validatorID]
	if !ok {
		return false
	}

	_, ok = vr.consensusGroup[validatorID]
	return ok
}

// IsNodeLeader returns if the validator is the current leader or not.
func (vr *validatorRegistry) IsNodeLeader(validatorID string) bool {
	pubKey, ok := vr.validators[validatorID]
	if !ok {
		return false
	}

	_, ok = vr.consensusGroup[validatorID]
	if !ok {
		return false
	}

	return bytes.Equal(pubKey, vr.currentLeader)
}

// GenerateConsensusGroup generates the consensus group.
func (vr *validatorRegistry) GenerateConsensusGroup(roundKey data.RoundKey) error {
	return nil
}

// ConsensusGroup returns the current consensus group.
func (vr *validatorRegistry) ConsensusGroup() [][]byte {
	group := make([][]byte, 0, len(vr.consensusGroup))
	for _, v := range vr.consensusGroup {
		group = append(group, v)
	}

	return group
}

// ConsensusGroupSize returns the current consensus group size.
func (vr *validatorRegistry) ConsensusGroupSize() uint64 {
	return uint64(len(vr.consensusGroup))
}
