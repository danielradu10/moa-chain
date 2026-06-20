package validators

import (
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"math"

	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/state"
)

type consensusSelector struct {
	currentConsensusGroup []string
	currentLeader         string
	logger                *slog.Logger
}

func NewConsensusSelector(loggers ...*slog.Logger) *consensusSelector {
	return &consensusSelector{
		currentConsensusGroup: make([]string, 0),
		currentLeader:         "",
		logger:                logging.FromOptional(loggers...),
	}
}

// SelectConsensusGroupMiniRoundOne selects the consensus group for the first mini-round one.
func (c *consensusSelector) SelectConsensusGroupMiniRoundOne(
	blockchainState state.BlockchainState,
	validators []*Validator,
	roundKey data.RoundKey,
) ([]string, error) {
	c.logger.Info("validators.SelectConsensusGroup started", "roundKey", roundKey, "numValidators", len(validators))

	provider, err := NewRandomnessProvider(blockchainState)
	if err != nil {
		c.logger.Error("validators.SelectConsensusGroup failed to create randomness provider", "roundKey", roundKey, "error", err)
		return nil, err
	}

	selectionSeed, err := provider.SelectionSeed(roundKey)
	if err != nil {
		c.logger.Error("validators.SelectConsensusGroup failed to get selection seed", "roundKey", roundKey, "error", err)
		return nil, err
	}
	c.logger.Debug("validators.SelectConsensusGroup got selection seed", "roundKey", roundKey, "seedLen", len(selectionSeed))

	return c.selectConsensusGroupMiniRoundOne(validators, selectionSeed)
}

func (c *consensusSelector) selectConsensusGroupMiniRoundOne(validators []*Validator, selectionSeed []byte) ([]string, error) {
	expandedList, err := c.createExpandedListMiniRoundOne(validators)
	if err != nil {
		c.logger.Error("validators.SelectConsensusGroup failed to create expanded list", "roundKey", roundKey, "error", err)
		return nil, err
	}
	c.logger.Debug(
		"validators.SelectConsensusGroup created expanded validator list",
		"roundKey", roundKey,
		"expandedListSize", len(expandedList),
		"numValidatorGroups", countValidatorGroups(expandedList),
	)

	consensusGroupDimension := len(validators) / 2
	consensusGroup, err := c.selectUniqueFromExpandedList(selectionSeed, expandedList, uint64(consensusGroupDimension))
	if err != nil {
		c.logger.Error("validators.SelectConsensusGroup failed to select consensus group", "roundKey", roundKey, "groupSize", consensusGroupDimension, "error", err)
		return nil, err
	}

	c.currentConsensusGroup = consensusGroup
	c.currentLeader = consensusGroup[0]
	c.logger.Info(
		"validators.SelectConsensusGroup finished",
		"roundKey", roundKey,
		"consensusGroup", consensusGroup,
		"leaderID", c.currentLeader,
	)

	return consensusGroup, nil
}

func (c *consensusSelector) createExpandedListMiniRoundOne(validators []*Validator) ([]string, error) {
	expandedList := make([]string, 0, len(validators))
	for _, validator := range validators {
		globalScore := int(math.Round(validator.GlobalScore()))
		c.logger.Debug(
			"validators.createExpandedList adding validator",
			"validatorID", validator.PublicID(),
			"globalScore", validator.GlobalScore(),
			"expandedWeight", globalScore,
		)

		for i := 0; i < globalScore; i++ {
			expandedList = append(expandedList, validator.PublicID())
		}
	}

	return expandedList, nil
}

// SelectConsensusGroupMiniRoundTwo select the consensus group for the second mini-round taking into consideration the frequency map resulted from the first mini-round.
func (c *consensusSelector) SelectConsensusGroupMiniRoundTwo(blockchainState state.BlockchainState, validators []*Validator, roundKey data.RoundKey, frequencyMap map[string]uint64) ([]string, error) {
	provider, err := NewRandomnessProvider(blockchainState)
	if err != nil {
		return nil, err
	}

	selectionSeed, err := provider.SelectionSeed(roundKey)
	if err != nil {
		return nil, err
	}

	return c.selectConsensusGroupMiniRoundTwo(validators, selectionSeed, frequencyMap)
}

func (c *consensusSelector) selectConsensusGroupMiniRoundTwo(validators []*Validator, selectionSeed []byte, frequencyMap map[string]uint64) ([]string, error) {
	totalFreq := uint64(0)
	for _, frequency := range frequencyMap {
		totalFreq += frequency
	}

	if totalFreq == 0 {
		return nil, ErrTotalFreqIsZero
	}

	weights := make(map[string]uint64)
	for subdomain, frequency := range frequencyMap {
		weights[subdomain] = uint64(math.Round(float64(frequency) / float64(totalFreq)))
	}

	consensusGroupDimension := len(validators) / 2
	expandedList := c.createExpandedListMiniRoundTwo(validators, weights)
	return c.selectUniqueFromExpandedList(selectionSeed, expandedList, uint64(consensusGroupDimension))
}

func (c *consensusSelector) createExpandedListMiniRoundTwo(validators []*Validator, weights map[string]uint64) []string {
	expandedList := make([]string, 0, len(validators))
	for _, validator := range validators {
		totalScore := c.calculateGlobalScoreOnCanonicalSubdomains(validator, weights)

		for i := uint64(0); i < totalScore; i++ {
			expandedList = append(expandedList, validator.PublicID())
		}
	}

	return expandedList
}

func (c *consensusSelector) calculateGlobalScoreOnCanonicalSubdomains(validator *Validator, wightsOfFreqMap map[string]uint64) uint64 {
	subdomainScores := validator.SubdomainScores()

	totalScore := uint64(0)
	for subdomain := range wightsOfFreqMap {
		totalScore += wightsOfFreqMap[subdomain] * uint64(subdomainScores[subdomain])
	}

	return totalScore
}

func (c *consensusSelector) selectUniqueFromExpandedList(
	seed []byte,
	expandedList []string,
	groupSize uint64,
) ([]string, error) {
	if len(seed) == 0 {
		c.logger.Error("validators.selectUniqueFromExpandedList got nil selection seed")
		return nil, ErrNilSelectionSeed
	}

	if len(expandedList) == 0 {
		c.logger.Error("validators.selectUniqueFromExpandedList got empty expanded list")
		return nil, ErrEmptyExpandedList
	}

	if groupSize == 0 {
		c.logger.Error("validators.selectUniqueFromExpandedList got invalid consensus group size", "groupSize", groupSize)
		return nil, ErrInvalidConsensusGroupSize
	}

	if uint64(countValidatorGroups(expandedList)) < groupSize {
		c.logger.Error(
			"validators.selectUniqueFromExpandedList does not have enough validators",
			"numValidatorGroups", countValidatorGroups(expandedList),
			"groupSize", groupSize,
		)
		return nil, ErrNotEnoughValidators
	}

	available := append([]string(nil), expandedList...)
	selected := make([]string, 0, groupSize)

	for selectionIndex := uint64(0); selectionIndex < groupSize; selectionIndex++ {
		if len(available) == 0 {
			c.logger.Error("validators.selectUniqueFromExpandedList ran out of validators", "selectionIndex", selectionIndex)
			return nil, ErrNotEnoughValidatorsSelected
		}

		index := c.deterministicIndex(seed, selectionIndex, len(available))

		validatorID := available[index]
		start, end := c.findValidatorInterval(available, index)

		selected = append(selected, validatorID)
		c.logger.Debug(
			"validators.selectUniqueFromExpandedList selected validator",
			"selectionIndex", selectionIndex,
			"validatorID", validatorID,
			"deterministicIndex", index,
			"availableBeforeRemoval", len(available),
		)

		available = c.removeInterval(available, start, end)
	}

	return selected, nil
}

func (c *consensusSelector) findValidatorInterval(expandedList []string, index int) (int, int) {
	validatorID := expandedList[index]

	start := index
	for start > 0 && expandedList[start-1] == validatorID {
		start--
	}

	end := index + 1
	for end < len(expandedList) && expandedList[end] == validatorID {
		end++
	}

	return start, end
}

func (c *consensusSelector) removeInterval(expandedList []string, start int, end int) []string {
	result := make([]string, 0, len(expandedList)-(end-start))

	result = append(result, expandedList[:start]...)
	result = append(result, expandedList[end:]...)

	return result
}

func (c *consensusSelector) deterministicIndex(seed []byte, selectionIndex uint64, listSize int) int {
	hasher := sha256.New()

	hasher.Write([]byte("MOA_CONSENSUS_SELECTION_INDEX_V1"))
	hasher.Write(seed)

	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], selectionIndex)
	hasher.Write(buffer[:])

	hash := hasher.Sum(nil)

	value := binary.BigEndian.Uint64(hash[:8])
	return int(value % uint64(listSize))
}

func countValidatorGroups(expandedList []string) int {
	if len(expandedList) == 0 {
		return 0
	}

	count := 1

	for i := 1; i < len(expandedList); i++ {
		if expandedList[i] != expandedList[i-1] {
			count++
		}
	}

	return count
}

func (c *consensusSelector) Leader() (string, error) {
	if c.currentLeader != "" {
		return c.currentLeader, nil
	}

	return "", ErrLeaderNotSelected
}

func (c *consensusSelector) ConsensusGroup() ([]string, error) {
	if c.currentConsensusGroup != nil {
		return c.currentConsensusGroup, nil
	}

	return nil, ErrConsensusGroupNotSelected
}

func (c *consensusSelector) ConsensusGroupSize() (uint64, error) {
	if c.currentConsensusGroup != nil {
		return uint64(len(c.currentConsensusGroup)), nil
	}

	return 0, nil
}
