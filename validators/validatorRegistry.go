package validators

import (
	"log/slog"
	"sort"

	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/state"
)

type validatorRegistry struct {
	cs             ConsensusSelector
	validators     map[string]*Validator
	consensusGroup map[string]struct{}
	logger         *slog.Logger
}

// NewValidatorRegistry creates a new validator registry.
func NewValidatorRegistry(
	cs ConsensusSelector,
	loggers ...*slog.Logger,
) *validatorRegistry {
	return &validatorRegistry{
		validators:     make(map[string]*Validator),
		cs:             cs,
		consensusGroup: make(map[string]struct{}),
		logger:         logging.FromOptional(loggers...),
	}
}

// Register registers a new validator.
func (vr *validatorRegistry) Register(validatorID string, validator *Validator) error {
	vr.validators[validatorID] = validator
	vr.logger.Info("validators.Register registered validator", "validatorID", validatorID, "numValidators", len(vr.validators))
	return nil
}

// GetPublicKey returns the public key of a validatorID.
func (vr *validatorRegistry) GetPublicKey(validatorID string) ([]byte, error) {
	_, ok := vr.validators[validatorID]
	if !ok {
		vr.logger.Error("validators.GetPublicKey failed because validator is unknown", "validatorID", validatorID)
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

// GenerateConsensusGroupMiniRoundOne generates the consensus group.
func (vr *validatorRegistry) GenerateConsensusGroupMiniRoundOne(
	blockchainState state.BlockchainState,
	roundKey data.RoundKey,
) error {
	vr.logger.Info("validators.GenerateConsensusGroupMiniRoundOne started", "roundKey", roundKey, "numValidators", len(vr.validators))

	ids := make([]string, 0, len(vr.validators))
	for id := range vr.validators {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	validators := make([]*Validator, 0, len(ids))
	for _, id := range ids {
		validators = append(validators, vr.validators[id])
	}

	consensusGroup, err := vr.cs.SelectConsensusGroupMiniRoundOne(blockchainState, validators, roundKey)
	if err != nil {
		vr.logger.Error("validators.GenerateConsensusGroupMiniRoundOne failed", "roundKey", roundKey, "error", err)
		return err
	}

	vr.consensusGroup = make(map[string]struct{})
	for _, validator := range consensusGroup {
		vr.consensusGroup[validator] = struct{}{}
	}

	leader, leaderErr := vr.cs.Leader()
	if leaderErr != nil {
		vr.logger.Info("validators.GenerateConsensusGroupMiniRoundOne finished", "roundKey", roundKey, "consensusGroup", consensusGroup)
		return nil
	}

	vr.logger.Info("validators.GenerateConsensusGroupMiniRoundOne finished", "roundKey", roundKey, "consensusGroup", consensusGroup, "leaderID", leader)
	return nil
}

// GenerateConsensusGroupMiniRoundTwo generates the consensus group of the second mini-round.
func (vr *validatorRegistry) GenerateConsensusGroupMiniRoundTwo(blockchainState state.BlockchainState, roundKey data.RoundKey, frequencyMap map[string]uint64) error {
	vr.logger.Info("validators.GenerateConsensusGroupMiniRoundTwo started", "roundKey", roundKey, "numValidators", len(vr.validators))

	ids := make([]string, 0, len(vr.validators))
	for id := range vr.validators {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	validators := make([]*Validator, 0, len(ids))
	for _, id := range ids {
		validators = append(validators, vr.validators[id])
	}

	consensusGroup, err := vr.cs.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)
	if err != nil {
		vr.logger.Error("validators.GenerateConsensusGroupMiniRoundTwo failed", "roundKey", roundKey, "error", err)
		return err
	}

	vr.consensusGroup = make(map[string]struct{})
	for _, validator := range consensusGroup {
		vr.consensusGroup[validator] = struct{}{}
	}

	leader, leaderErr := vr.cs.Leader()
	if leaderErr != nil {
		vr.logger.Info("validators.GenerateConsensusGroupMiniRoundTwo finished", "roundKey", roundKey, "consensusGroup", consensusGroup)
		return nil
	}

	vr.logger.Info("validators.GenerateConsensusGroupMiniRoundTwo finished", "roundKey", roundKey, "consensusGroup", consensusGroup, "leaderID", leader)
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

	sort.Strings(ids)
	return ids
}
