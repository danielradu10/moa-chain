package validators

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestValidatorRegistry_GenerateConsensusGroupMiniRoundTwo(t *testing.T) {
	t.Parallel()

	selector := NewConsensusSelector()
	registry := NewValidatorRegistry(selector)

	validatorsToRegister := createMiniRoundTwoSelectionValidators()
	for _, validator := range validatorsToRegister {
		err := registry.Register(validator.PublicID(), validator)
		require.NoError(t, err)
	}

	roundKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	frequencyMap := map[string]uint64{
		"security":  70,
		"databases": 30,
	}

	err := registry.GenerateConsensusGroupMiniRoundTwo(createSelectionTestBlockchainState(), roundKey, frequencyMap)

	require.NoError(t, err)

	consensusGroup, err := registry.ConsensusGroup()
	require.NoError(t, err)
	require.Equal(t, []string{"validator-3", "validator-1"}, consensusGroup)

	leader, err := registry.LeaderOfConsensusGroup()
	require.NoError(t, err)
	require.Equal(t, "validator-3", leader)

	require.True(t, registry.IsValidatorInConsensusGroup("validator-3"))
	require.True(t, registry.IsValidatorInConsensusGroup("validator-1"))
	require.False(t, registry.IsValidatorInConsensusGroup("validator-2"))
}
