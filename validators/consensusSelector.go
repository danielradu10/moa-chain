package validators

import (
	"crypto/sha256"
	"encoding/binary"
	"math"

	"moa-chain/data"
	"moa-chain/state"
)

type consensusSelector struct {
	currentConsensusGroup []string
	currentLeader         string
}

func NewConsensusSelector() *consensusSelector {
	return &consensusSelector{
		currentConsensusGroup: make([]string, 0),
		currentLeader:         "",
	}
}

func (c *consensusSelector) SelectConsensusGroup(
	blockchainState state.BlockchainState,
	validators []*Validator,
	roundKey data.RoundKey,
) ([]string, error) {
	provider, err := NewRandomnessProvider(blockchainState)
	if err != nil {
		return nil, err
	}

	selectionSeed, err := provider.SelectionSeed(roundKey)
	if err != nil {
		return nil, err
	}

	expandedList, err := c.createExpandedList(validators)
	if err != nil {
		return nil, err
	}

	consensusGroupDimension := len(validators) / 2
	consensusGroup, err := c.selectUniqueFromExpandedList(selectionSeed, expandedList, uint64(consensusGroupDimension))
	if err != nil {
		return nil, err
	}

	c.currentConsensusGroup = consensusGroup
	c.currentLeader = consensusGroup[0]

	return consensusGroup, nil
}

func (c *consensusSelector) createExpandedList(validators []*Validator) ([]string, error) {
	expandedList := make([]string, 0, len(validators))
	for _, validator := range validators {
		globalScore := int(math.Round(validator.GlobalScore()))

		for i := 0; i < globalScore; i++ {
			expandedList = append(expandedList, validator.PublicID())
		}
	}

	return expandedList, nil
}

func (c *consensusSelector) selectUniqueFromExpandedList(
	seed []byte,
	expandedList []string,
	groupSize uint64,
) ([]string, error) {
	if len(seed) == 0 {
		return nil, ErrNilSelectionSeed
	}

	if len(expandedList) == 0 {
		return nil, ErrEmptyExpandedList
	}

	if groupSize == 0 {
		return nil, ErrInvalidConsensusGroupSize
	}

	if uint64(countValidatorGroups(expandedList)) < groupSize {
		return nil, ErrNotEnoughValidators
	}

	available := append([]string(nil), expandedList...)
	selected := make([]string, 0, groupSize)

	for selectionIndex := uint64(0); selectionIndex < groupSize; selectionIndex++ {
		if len(available) == 0 {
			return nil, ErrNotEnoughValidatorsSelected
		}

		index := c.deterministicIndex(seed, selectionIndex, len(available))

		validatorID := available[index]
		start, end := c.findValidatorInterval(available, index)

		selected = append(selected, validatorID)

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
