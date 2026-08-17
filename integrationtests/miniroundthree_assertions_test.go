package integrationtests

import (
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func waitForMiniRoundThreeFinalization(
	t *testing.T,
	nodes []*integrationTestNode,
	roundKey data.RoundKey,
) {
	t.Helper()

	var protocolErr error
	require.Eventually(t, func() bool {
		if protocolErr = firstScenarioError(nodes); protocolErr != nil {
			return true
		}

		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRThree(roundKey); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, protocolErr)
}

func waitForAllNodesMR2Finalization(
	t *testing.T,
	nodes []*integrationTestNode,
	roundKey data.RoundKey,
) {
	t.Helper()

	var protocolErr error
	require.Eventually(t, func() bool {
		if protocolErr = firstScenarioError(nodes); protocolErr != nil {
			return true
		}
		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, protocolErr)
}

func requireMR3ScenarioFinalizedState(
	t *testing.T,
	scenario miniRoundThreeScenario,
	nodes []*integrationTestNode,
	committees mr3ScenarioCommittees,
	mr3Key data.RoundKey,
) {
	t.Helper()

	firstMR3Block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRThree(mr3Key)
	require.NoError(t, err)
	require.Len(t, firstMR3Block.FinalAnswers, len(scenario.Expected.FinalAnswers))

	expectedByTxHash := make(map[string]scenarioExpectedFinalAnswer, len(scenario.Expected.FinalAnswers))
	for _, expected := range scenario.Expected.FinalAnswers {
		expectedByTxHash[expected.TxHash] = expected
	}

	for _, finalAnswer := range firstMR3Block.FinalAnswers {
		txHash := string(finalAnswer.TxHash)
		expected, exists := expectedByTxHash[txHash]
		require.Truef(t, exists, "unexpected final answer for tx %s", txHash)
		require.Equal(t, expected.Status, finalAnswer.Status)
		if expected.Status == data.FinalAnswerStatusSynthesized {
			require.Equal(t, expected.Answer, finalAnswer.Answer)
		}
	}

	if len(scenario.Expected.SynthesisVoters) > 0 {
		mr3LeaderID := committees.miniRoundThree[0]
		leaderNode := scenarioNodeByID(t, nodes, mr3LeaderID)
		synthVotes, getErr := leaderNode.roundState.GetSynthesisVotes(mr3Key)
		require.NoError(t, getErr)

		mr3Quorum := (2 * scenario.Network.CommitteeSize) / 3
		require.Len(t, synthVotes, mr3Quorum)

		expectedVoterIDs := scenarioIDsForRoles(t, committees.miniRoundThree, scenario.Expected.SynthesisVoters)
		sort.Strings(expectedVoterIDs)
		actualVoterIDs := make([]string, 0, len(synthVotes))
		for _, vote := range synthVotes {
			actualVoterIDs = append(actualVoterIDs, vote.VoterID)
		}
		sort.Strings(actualVoterIDs)
		require.Equal(t, expectedVoterIDs, actualVoterIDs)
	}

	requireAllMR3NodesFinalizedSameBlock(t, nodes, firstMR3Block, mr3Key)
}

func requireAllMR3NodesFinalizedSameBlock(
	t *testing.T,
	nodes []*integrationTestNode,
	firstMR3Block *data.BlockOnChain,
	mr3Key data.RoundKey,
) {
	t.Helper()

	for _, node := range nodes {
		block, err := node.blockFinalizer.GetFinalizedBlockInMRThree(mr3Key)
		require.NoError(t, err)
		require.Len(t, block.FinalAnswers, len(firstMR3Block.FinalAnswers))

		for i, expected := range firstMR3Block.FinalAnswers {
			actual := block.FinalAnswers[i]
			require.Equal(t, expected.Status, actual.Status)
			require.Equal(t, expected.Answer, actual.Answer)
		}
	}
}
