package integrationtests

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

// Add a scenario name here to run another fixture through the same MR1 -> MR2
// protocol runner. Scenario-specific behavior belongs in its fixture, not in
// this test function.
func TestMiniRoundOneToMiniRoundTwoScenarios(t *testing.T) {
	scenarios := []string{
		"unanimous_correct",
		"insufficient_correct_answers",
		// Scenario 3 depends on text-visible disputed answers because the fake
		// judge preserves the production information boundary: it classifies by
		// transaction hash and answer text, not by hidden producer role.
		// It also encodes the current fallback grouping policy: Correct needs
		// full quorum; once Correct misses quorum, ties among Hallucination,
		// Malicious, and Wrong are resolved by the protocol's fallback order.
		"four_category_disagreement",
		// Scenario 4 keeps the prompt-injection text as answer data. The
		// deterministic fake judge matches the literal answer text; it must not
		// interpret fixture strings as test or protocol instructions.
		"prompt_injection_answer",
		// Scenario 5 puts the failing judge after the first seven valid voters in
		// delivery order. This checks that a judge execution error produces no
		// partial signed vote without blocking a valid certificate.
		"judge_execution_error",
		// Scenario 6 models malformed judge responses as no-vote failures. The
		// malformed judge is delayed after seven valid voters so the test can
		// assert that malformed output is absent from the certificate.
		"malformed_judge_response",
		// Scenario 7 mutates a signed vote at the network boundary. The judge
		// creates a valid vote first; the transport fault corrupts its signature
		// so the leader rejects Byzantine input and continues collecting votes.
		"byzantine_signed_classification_vote",
		// Scenario 8 changes only transport arrival order. Message contents stay
		// valid so the expected result should match the unanimous-correct baseline
		// after evidence and certificate canonicalization.
		"shuffled_valid_message_arrival",
		// Scenario 9 mutates the leader's broadcast certificate in place. This
		// models a malicious leader broadcasting a non-canonical certificate, so
		// both receivers and the leader's subsequent local verification see the
		// same tampered artifact instead of letting the leader finalize an
		// unbroadcast valid certificate.
		"leader_reorders_certificate_votes",
		// Scenario 10 keeps the leader-provided groups unchanged but removes one
		// signed vote from the broadcast certificate. Validators must reject the
		// insufficient evidence instead of trusting derived classifications.
		"leader_omits_certificate_vote",
		// Scenario 11 mutates answer evidence at the leader broadcast boundary.
		// Validators reject the evidence before judging, so no classification vote
		// should be produced from the invalid artifact.
		"invalid_answer_evidence",
		// Scenario 12 drops enough classification votes to leave the leader with
		// six valid votes, then injects the collection timeout for that step. The
		// timeout is sent only after those six votes are stored to avoid racing the
		// protocol state machine.
		"classification_quorum_timeout",
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
	// The protocol now aggregates all G (CommitteeSize) votes before applying the
	// Q (Quorum) threshold for label frequency. Frequencies are therefore G×txCount
	// per label, not Q×txCount.
	frequencies := scenarioSubdomainFrequencies(scenario, uint64(scenario.Network.CommitteeSize))
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

	if scenario.Delivery.TimeoutStep != 0 {
		scheduleScenarioTimeout(t, scenario, nodes, committees, inboxes, miniRoundTwoKey)
	}

	if !scenario.Expected.RoundFinalized {
		requireScenarioDoesNotFinalize(t, scenario, nodes, miniRoundOneKey, miniRoundTwoKey)
		stopNodes()
		requireMiniRoundOneFinalizedOnAllNodes(t, nodes, miniRoundOneKey)
		writeScenarioRejectedResult(t, scenario, committees, nodes, miniRoundOneKey, miniRoundTwoKey)
		return
	}

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

func scheduleScenarioTimeout(
	t *testing.T,
	scenario miniRoundTwoScenario,
	nodes []*integrationTestNode,
	committees scenarioCommittees,
	inboxes []chan data.RoundEvent,
	roundKey data.RoundKey,
) {
	t.Helper()

	leaderID := committees.miniRoundTwo[0]
	leaderIndex := scenarioValidatorIndex(leaderID)
	leaderNode := scenarioNodeByID(t, nodes, leaderID)
	go func() {
		deadline := time.After(5 * time.Second)
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			votes, err := leaderNode.roundState.GetAnswerClassificationVotes(roundKey)
			if err == nil && len(votes) == scenario.Network.Quorum-1 {
				inboxes[leaderIndex] <- data.RoundEvent{
					Type: data.TimeoutEvent,
					Timeout: &data.RoundTimeout{
						RoundKey: roundKey,
						Step:     scenario.Delivery.TimeoutStep,
					},
				}
				return
			}

			select {
			case <-deadline:
				t.Errorf("timed out waiting to inject scenario timeout")
				return
			case <-ticker.C:
			}
		}
	}()
}

func requireScenarioDoesNotFinalize(
	t *testing.T,
	scenario miniRoundTwoScenario,
	nodes []*integrationTestNode,
	miniRoundOneKey data.RoundKey,
	miniRoundTwoKey data.RoundKey,
) {
	t.Helper()

	err := waitForScenarioError(t, nodes, scenario.Expected.ErrorContains)
	require.Error(t, err)
	requireMiniRoundOneFinalizedOnAllNodes(t, nodes, miniRoundOneKey)
	require.Equal(t, scenario.Expected.FinalizedNodes, finalizedMiniRoundTwoNodeCount(nodes, miniRoundTwoKey))
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
			false,
			0,
			validators.CommitteeStrategyHalf,
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

func waitForScenarioError(
	t *testing.T,
	nodes []*integrationTestNode,
	expectedError string,
) error {
	t.Helper()

	var observed error
	require.Eventually(t, func() bool {
		for _, node := range nodes {
			select {
			case err := <-node.loop.Errors():
				if err == nil {
					continue
				}
				if strings.Contains(err.Error(), expectedError) {
					observed = err
					return true
				}
				observed = errors.Join(observed, fmt.Errorf("%s: %w", node.id, err))
			default:
			}
		}
		return false
	}, 10*time.Second, 10*time.Millisecond)

	return observed
}

func requireMiniRoundOneFinalizedOnAllNodes(
	t *testing.T,
	nodes []*integrationTestNode,
	roundKey data.RoundKey,
) {
	t.Helper()

	for _, node := range nodes {
		_, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		require.NoError(t, err)
	}
}

func finalizedMiniRoundTwoNodeCount(nodes []*integrationTestNode, roundKey data.RoundKey) int {
	finalized := 0
	for _, node := range nodes {
		if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey); err == nil {
			finalized++
		}
	}
	return finalized
}

func scenarioValidatorIndex(validatorID string) int {
	var index int
	_, _ = fmt.Sscanf(validatorID, "validator-%d", &index)
	return index - 1
}
