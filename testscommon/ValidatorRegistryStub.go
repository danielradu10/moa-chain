package testscommon

import (
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validators"
)

type ValidatorRegistryStub struct {
	RegisteredValidators    map[string]bool
	ConsensusValidators     map[string]bool
	PublicKeysByValidatorID map[string][]byte
	GetPublicKeyErr         error
	LeaderID                string
	ConsensusGroupSizeValue uint64
	ConsensusGroupValue     []string
	ValidatorsIDs           []string
}

func (stub *ValidatorRegistryStub) Register(validatorID string, validator *validators.Validator) error {
	return nil
}

func (stub *ValidatorRegistryStub) GetPublicKey(validatorID string) ([]byte, error) {
	if stub.GetPublicKeyErr != nil {
		return nil, stub.GetPublicKeyErr
	}

	return stub.PublicKeysByValidatorID[validatorID], nil
}

func (stub *ValidatorRegistryStub) IsValidatorRegistered(validatorID string) bool {
	return stub.RegisteredValidators[validatorID]
}

func (stub *ValidatorRegistryStub) IsValidatorInConsensusGroup(validatorID string) bool {
	return stub.ConsensusValidators[validatorID]
}

func (stub *ValidatorRegistryStub) IsNodeLeader(validatorID string) bool {
	return validatorID == stub.LeaderID
}

func (stub *ValidatorRegistryStub) LeaderOfConsensusGroup() (string, error) {
	return stub.LeaderID, nil
}

func (stub *ValidatorRegistryStub) GenerateConsensusGroupMiniRoundOne(blockchainState state.BlockchainState, roundKey data.RoundKey) error {
	return nil
}

func (stub *ValidatorRegistryStub) GenerateConsensusGroupMiniRoundTwo(blockchainState state.BlockchainState, roundKey data.RoundKey, frequencyMap map[string]uint64) error {
	return nil
}

func (stub *ValidatorRegistryStub) ConsensusGroup() ([]string, error) {
	return stub.ConsensusGroupValue, nil
}

func (stub *ValidatorRegistryStub) ConsensusGroupSize() (uint64, error) {
	return stub.ConsensusGroupSizeValue, nil
}

func (stub *ValidatorRegistryStub) GetValidatorsIDs() []string {
	return stub.ValidatorsIDs
}
