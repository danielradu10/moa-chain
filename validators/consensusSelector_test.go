package validators

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
)

func TestConsensusSelector_SelectConsensusGroupMiniRoundTwo(t *testing.T) {
	t.Parallel()

	t.Run("should select deterministic consensus group and leader from subdomain scores", func(t *testing.T) {
		t.Parallel()

		selector := NewConsensusSelector()
		blockchainState := createSelectionTestBlockchainState()
		roundKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		validators := createMiniRoundTwoSelectionValidators()
		frequencyMap := map[string]uint64{
			"security":  70,
			"databases": 30,
		}

		consensusGroup, err := selector.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)

		require.NoError(t, err)
		require.Equal(t, []string{"validator-3", "validator-1"}, consensusGroup)

		storedConsensusGroup, err := selector.ConsensusGroup()
		require.NoError(t, err)
		require.Equal(t, consensusGroup, storedConsensusGroup)

		leader, err := selector.Leader()
		require.NoError(t, err)
		require.Equal(t, consensusGroup[0], leader)
	})

	t.Run("should be deterministic for the same inputs", func(t *testing.T) {
		t.Parallel()

		blockchainState := createSelectionTestBlockchainState()
		roundKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		validators := createMiniRoundTwoSelectionValidators()
		frequencyMap := map[string]uint64{
			"security":  70,
			"databases": 30,
		}

		firstSelector := NewConsensusSelector()
		firstConsensusGroup, err := firstSelector.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)
		require.NoError(t, err)

		secondSelector := NewConsensusSelector()
		secondConsensusGroup, err := secondSelector.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)
		require.NoError(t, err)

		require.Equal(t, firstConsensusGroup, secondConsensusGroup)
	})

	t.Run("should return ErrTotalFreqIsZero when frequency sum is zero", func(t *testing.T) {
		t.Parallel()

		selector := NewConsensusSelector()
		blockchainState := createSelectionTestBlockchainState()
		roundKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		validators := createMiniRoundTwoSelectionValidators()
		frequencyMap := map[string]uint64{
			"security":  0,
			"databases": 0,
		}

		consensusGroup, err := selector.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)

		require.Nil(t, consensusGroup)
		require.Equal(t, ErrTotalFreqIsZero, err)
	})

	t.Run("should return ErrEmptyExpandedList when all effective scores are zero", func(t *testing.T) {
		t.Parallel()

		selector := NewConsensusSelector()
		blockchainState := createSelectionTestBlockchainState()
		roundKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		validators := []*Validator{
			createMiniRoundTwoSelectionValidator("validator-1", map[string]int{"security": 0}),
			createMiniRoundTwoSelectionValidator("validator-2", map[string]int{"security": 0}),
		}
		frequencyMap := map[string]uint64{
			"security": 100,
		}

		consensusGroup, err := selector.SelectConsensusGroupMiniRoundTwo(blockchainState, validators, roundKey, frequencyMap)

		require.Nil(t, consensusGroup)
		require.Equal(t, ErrEmptyExpandedList, err)
	})
}

func TestConsensusSelector_calculateGlobalScoreOnCanonicalSubdomains(t *testing.T) {
	t.Parallel()

	selector := NewConsensusSelector()
	validator := createMiniRoundTwoSelectionValidator(
		"validator-1",
		map[string]int{
			"security":  10,
			"databases": 5,
		},
	)
	weights := map[string]uint64{
		"security":  70,
		"databases": 30,
	}

	score := selector.calculateGlobalScoreOnCanonicalSubdomains(validator, weights)

	require.Equal(t, uint64(850), score)
}

func createMiniRoundTwoSelectionValidators() []*Validator {
	return []*Validator{
		createMiniRoundTwoSelectionValidator(
			"validator-1",
			map[string]int{
				"security":  10,
				"databases": 1,
			},
		),
		createMiniRoundTwoSelectionValidator(
			"validator-2",
			map[string]int{
				"security":  1,
				"databases": 10,
			},
		),
		createMiniRoundTwoSelectionValidator(
			"validator-3",
			map[string]int{
				"security":  6,
				"databases": 6,
			},
		),
		createMiniRoundTwoSelectionValidator(
			"validator-4",
			map[string]int{
				"security":  1,
				"databases": 1,
			},
		),
	}
}

func createMiniRoundTwoSelectionValidator(publicID string, scores map[string]int) *Validator {
	validator := NewValidator(publicID, []byte(publicID+"-public-key"), 100)
	validator.SetSubdomainScores(scores)

	return validator
}

func createSelectionTestBlockchainState() *testscommon.BlockchainStateStub {
	return &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: &data.BlockHeader{
			HeaderHash: []byte("selection-test-current-header-hash"),
		},
	}
}
