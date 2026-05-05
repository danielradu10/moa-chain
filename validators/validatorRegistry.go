package validators

import (
	"moa-chain/data"
	"moa-chain/state"
)

type validatorRegistry struct {
	cs             ConsensusSelector
	validators     map[string]*Validator
	consensusGroup map[string]struct{}
}

// NewValidatorRegistry creates a new validator registry.
func NewValidatorRegistry(
	cs ConsensusSelector,
) *validatorRegistry {
	return &validatorRegistry{
		validators:     make(map[string]*Validator),
		cs:             cs,
		consensusGroup: make(map[string]struct{}),
	}
}

// GetPublicKey returns the public key of a validatorID.
func (vr *validatorRegistry) GetPublicKey(validatorID string) ([]byte, error) {
	_, ok := vr.validators[validatorID]
	if !ok {
		return nil, ErrInvalidValidator
	}

	return vr.validators[validatorID].PublicKey(), nil
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
	currentLeader, err := vr.cs.Leader()
	if err != nil {
		return false
	}

	return validatorID == currentLeader
}

// GenerateConsensusGroup generates the consensus group.
func (vr *validatorRegistry) GenerateConsensusGroup(blockchainState state.BlockchainState, roundKey data.RoundKey) error {
	validators := make([]*Validator, 0, len(vr.validators))
	for _, validator := range vr.validators {
		validators = append(validators, validator)
	}

	consensusGroup, err := vr.cs.SelectConsensusGroup(blockchainState, validators, roundKey)
	if err != nil {
		return err
	}

	vr.consensusGroup = make(map[string]struct{})
	for _, validator := range consensusGroup {
		vr.consensusGroup[validator] = struct{}{}
	}

	return nil
}

// ConsensusGroup returns the current consensus group.
func (vr *validatorRegistry) ConsensusGroup() ([]string, error) {
	return vr.cs.ConsensusGroup()
}

// LeaderOfConsensusGroup returns the current leader of the consensus group.
func (vr *validatorRegistry) LeaderOfConsensusGroup() (string, error) {
	return vr.cs.Leader()
}

// ConsensusGroupSize returns the current consensus group size.
func (vr *validatorRegistry) ConsensusGroupSize() (uint64, error) {
	return vr.cs.ConsensusGroupSize()
}

// GetValidatorsIDs returns all the registered ids
func (vr *validatorRegistry) GetValidatorsIDs() []string {
	ids := make([]string, 0, len(vr.validators))
	for k := range vr.validators {
		ids = append(ids, k)
	}

	return ids
}
