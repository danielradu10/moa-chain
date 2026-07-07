package integrationtests

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
)

func newMiniRoundTwoScenarioNetwork(
	t *testing.T,
	inboxes []chan data.RoundEvent,
	committees scenarioCommittees,
	scenario miniRoundTwoScenario,
) *testscommon.OrderedBroadcasterNetworkStub {
	t.Helper()

	// These are fixture-based scenario tests, not property tests. The fixture
	// asserts exact evidence producers, classification voters, counts, and final
	// statuses. Mini-round two accepts the first valid quorum, so unrestricted
	// goroutine scheduling could produce a different valid quorum on each run and
	// make the test pass or fail based on timing. Ordering pins the first quorum
	// to the one described by the fixture, so the test verifies the intended edge
	// case instead of accepting any valid protocol outcome.
	return testscommon.NewOrderedBroadcasterNetworkStub(
		scenarioInboxesByValidator(inboxes),
		[]testscommon.OrderedDeliveryRule{
			{
				MessageType: data.ExecutedPromptsMessage,
				SenderOrder: scenarioIDsForRoles(
					t,
					committees.miniRoundTwo,
					scenario.Delivery.ProducerOrder,
				),
			},
			{
				MessageType: data.AnswerClassificationVoteConsensusMessage,
				// Fault scenarios can place a failing judge after the first seven
				// valid voters. The full role list remains explicit, but the first
				// quorum is not blocked by a validator that correctly sends no vote.
				SenderOrder: scenarioIDsForRoles(
					t,
					committees.miniRoundTwo,
					scenario.Delivery.JudgeOrder,
				),
				// The leader handles its own classification vote directly instead of
				// sending it through the broadcaster, so delivery starts at member-1.
				StartIndex: 1,
			},
		},
	)
}

func scenarioInboxesByValidator(inboxes []chan data.RoundEvent) map[string]chan data.RoundEvent {
	registeredInboxes := make(map[string]chan data.RoundEvent, len(inboxes))
	for index, inbox := range inboxes {
		registeredInboxes[fmt.Sprintf("validator-%d", index+1)] = inbox
	}
	return registeredInboxes
}

func startScenarioNodes(nodes []*integrationTestNode) []<-chan struct{} {
	done := make([]<-chan struct{}, 0, len(nodes))
	for _, node := range nodes {
		nodeDone := make(chan struct{})
		done = append(done, nodeDone)
		go func(currentNode *integrationTestNode) {
			defer close(nodeDone)
			currentNode.loop.Run()
		}(node)
	}
	return done
}

func firstScenarioError(nodes []*integrationTestNode) error {
	for _, node := range nodes {
		select {
		case err := <-node.loop.Errors():
			return fmt.Errorf("%s: %w", node.id, err)
		default:
		}
	}
	return nil
}

func scenarioNodeByID(t *testing.T, nodes []*integrationTestNode, id string) *integrationTestNode {
	t.Helper()
	for _, node := range nodes {
		if node.id == id {
			return node
		}
	}
	require.FailNow(t, "scenario node not found", id)
	return nil
}
