package integrationtests

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
)

// Add a scenario name here to run another fixture through the same MR1 -> MR2
// protocol runner. Scenario-specific behavior belongs in its fixture, not in
// this test function.
func TestMiniRoundOneToMiniRoundTwoScenarios(t *testing.T) {
	scenarios := []string{
		"unanimous_correct",
		"insufficient_correct_answers",
	}

	for _, scenarioName := range scenarios {
		t.Run(scenarioName, func(t *testing.T) {
			runMiniRoundTwoScenario(t, loadMiniRoundTwoScenario(t, scenarioName))
		})
	}
}

func runMiniRoundTwoScenario(t *testing.T, scenario miniRoundTwoScenario) {
	t.Helper()

	publicKeys, privateKeys := generateScenarioKeys(t, scenario.Network.RegisteredNodes)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := scenarioTransactions(scenario)
	frequencies := scenarioSubdomainFrequencies(scenario, uint64(scenario.Network.Quorum))
	committees := selectScenarioCommittees(t, registeredValidators, frequencies)
	require.Len(t, committees.miniRoundOne, scenario.Network.CommitteeSize)
	require.Len(t, committees.miniRoundTwo, scenario.Network.CommitteeSize)

	miniRoundTwoRoles := scenarioRolesByValidator(committees.miniRoundTwo)
	inboxes := makeScenarioInboxes(scenario.Network.RegisteredNodes)
	network := newMiniRoundTwoScenarioNetwork(t, inboxes, committees, scenario)

	nodes := createMiniRoundTwoScenarioNodes(
		t,
		scenario,
		registeredValidators,
		privateKeys,
		inboxes,
		miniRoundTwoRoles,
		network,
		transactions,
	)

	done := startScenarioNodes(nodes)
	stopNodes := stopScenarioNodesOnce(t, inboxes, done)
	t.Cleanup(stopNodes)

	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	startScenarioRound(inboxes, miniRoundOneKey)

	waitForMiniRoundTwoFinalization(t, nodes, miniRoundTwoKey)

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	firstMiniRoundOneBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
	require.NoError(t, err)
	firstMiniRoundTwoBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
	require.NoError(t, err)

	leaderID := committees.miniRoundTwo[0]
	leaderNode := scenarioNodeByID(t, nodes, leaderID)
	classificationVotes, err := leaderNode.roundState.GetAnswerClassificationVotes(miniRoundTwoKey)
	require.NoError(t, err)

	requireScenarioFinalizedState(
		t,
		scenario,
		nodes,
		committees,
		frequencies,
		firstMiniRoundOneBlock,
		firstMiniRoundTwoBlock,
		classificationVotes,
		miniRoundOneKey,
		miniRoundTwoKey,
	)

	writeScenarioResult(t, scenario, committees, firstMiniRoundOneBlock, firstMiniRoundTwoBlock, classificationVotes)
}

func createMiniRoundTwoScenarioNodes(
	t *testing.T,
	scenario miniRoundTwoScenario,
	registeredValidators scenarioValidators,
	privateKeys [][]byte,
	inboxes []chan data.RoundEvent,
	miniRoundTwoRoles map[string]string,
	network *testscommon.OrderedBroadcasterNetworkStub,
	transactions []data.Transaction,
) []*integrationTestNode {
	t.Helper()

	nodes := make([]*integrationTestNode, 0, scenario.Network.RegisteredNodes)
	for index := 0; index < scenario.Network.RegisteredNodes; index++ {
		validatorID := fmt.Sprintf("validator-%d", index+1)
		role := miniRoundTwoRoles[validatorID]
		if role == "" {
			role = "observer"
		}

		node := createNodeWithBroadcaster(
			t,
			validatorID,
			privateKeys[index],
			registeredValidators,
			inboxes[index],
			cloneTransactions(transactions),
			createScenarioAgent(t, scenario, role),
			network.BroadcasterForNode(validatorID),
		)
		nodes = append(nodes, node)
	}
	return nodes
}

func stopScenarioNodesOnce(
	t *testing.T,
	inboxes []chan data.RoundEvent,
	done []<-chan struct{},
) func() {
	t.Helper()

	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			for _, inbox := range inboxes {
				inbox <- data.RoundEvent{Type: data.StopEvent}
			}
			for _, nodeDone := range done {
				select {
				case <-nodeDone:
				case <-time.After(5 * time.Second):
					t.Errorf("timed out stopping scenario node")
				}
			}
		})
	}
}

func startScenarioRound(inboxes []chan data.RoundEvent, roundKey data.RoundKey) {
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: roundKey}
	}
}

func waitForMiniRoundTwoFinalization(
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
