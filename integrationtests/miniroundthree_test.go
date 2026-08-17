package integrationtests

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestMiniRoundOneToMiniRoundThreeScenarios(t *testing.T) {
	scenarios := []string{
		// All producers return correct answers, all evaluators approve the leader's
		// synthesis, and all three transactions finalize as SYNTHESIZED.
		"unanimous_synthesis_approval",
		// One transaction is INSUFFICIENT_CORRECT_ANSWERS in MR2 (four executors
		// return a wrong answer); the leader synthesizes only the two eligible
		// transactions and skips the insufficient one.
		"partial_tx_eligibility",
		// All three transactions are INSUFFICIENT_CORRECT_ANSWERS in MR2 (four
		// executors return a wrong answer for every transaction); the leader
		// broadcasts an empty proposal, validators approve vacuously, and all
		// three final answers are SKIPPED.
		"no_eligible_transactions",
	}

	for _, scenarioName := range scenarios {
		t.Run(scenarioName, func(t *testing.T) {
			runMiniRoundThreeScenario(t, loadMiniRoundThreeScenario(t, scenarioName))
		})
	}
}

func runMiniRoundThreeScenario(t *testing.T, scenario miniRoundThreeScenario) {
	t.Helper()

	publicKeys, privateKeys := generateScenarioKeys(t, scenario.Network.RegisteredNodes)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := mr3ScenarioTransactions(scenario)
	frequencies := computeMR3SubdomainFrequencies(scenario.Transactions, uint64(scenario.Network.CommitteeSize))
	committees := selectMR3ScenarioCommittees(t, registeredValidators, frequencies)
	require.Len(t, committees.miniRoundOne, scenario.Network.CommitteeSize)
	require.Len(t, committees.miniRoundTwo, scenario.Network.CommitteeSize)
	require.Len(t, committees.miniRoundThree, scenario.Network.CommitteeSize)

	inboxes := makeScenarioInboxes(scenario.Network.RegisteredNodes)
	network := newMiniRoundThreeScenarioNetwork(t, inboxes, committees, scenario)

	nodes := createMiniRoundThreeScenarioNodes(
		t,
		scenario,
		registeredValidators,
		privateKeys,
		inboxes,
		committees,
		network,
		transactions,
	)

	done := startScenarioNodes(nodes)
	stopNodes := stopScenarioNodesOnce(t, nodes, done)
	t.Cleanup(stopNodes)

	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	miniRoundThreeKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundThree)}
	startScenarioRound(inboxes, miniRoundOneKey)

	waitForMiniRoundThreeFinalization(t, nodes, miniRoundThreeKey)

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	firstMR1Block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
	require.NoError(t, err)
	firstMR2Block, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
	require.NoError(t, err)

	mr2LeaderID := committees.miniRoundTwo[0]
	mr2LeaderNode := scenarioNodeByID(t, nodes, mr2LeaderID)
	classificationVotes, err := mr2LeaderNode.roundState.GetAnswerClassificationVotes(miniRoundTwoKey)
	require.NoError(t, err)

	requireScenarioFinalizedState(
		t,
		mr3ScenarioAsMR2(scenario),
		nodes,
		scenarioCommittees{miniRoundOne: committees.miniRoundOne, miniRoundTwo: committees.miniRoundTwo},
		frequencies,
		firstMR1Block,
		firstMR2Block,
		classificationVotes,
		miniRoundOneKey,
		miniRoundTwoKey,
	)

	requireMR3ScenarioFinalizedState(t, scenario, nodes, committees, miniRoundThreeKey)
}

// mr3ScenarioAsMR2 converts the shared MR2 fields of a miniRoundThreeScenario
// into a miniRoundTwoScenario so existing MR2 assertion helpers can be reused.
func mr3ScenarioAsMR2(s miniRoundThreeScenario) miniRoundTwoScenario {
	return miniRoundTwoScenario{
		Network:      s.Network,
		Transactions: s.Transactions,
		Executors:    s.Executors,
		Judges:       s.Judges,
		Delivery: scenarioDeliveryFixture{
			ProducerOrder:                   s.Delivery.ProducerOrder,
			JudgeOrder:                      s.Delivery.JudgeOrder,
			AnswerEvidenceFaults:            s.Delivery.AnswerEvidenceFaults,
			ClassificationVoteFaults:        s.Delivery.ClassificationVoteFaults,
			ClassificationCertificateFaults: s.Delivery.ClassificationCertificateFaults,
		},
		Expected: scenarioExpectedFixture{
			RoundFinalized:          s.Expected.RoundFinalized,
			FinalizedNodes:          s.Expected.FinalizedNodes,
			ErrorContains:           s.Expected.ErrorContains,
			AnswerEvidenceProducers: s.Expected.AnswerEvidenceProducers,
			ClassificationVoters:    s.Expected.ClassificationVoters,
			Transactions:            s.Expected.Transactions,
		},
	}
}
