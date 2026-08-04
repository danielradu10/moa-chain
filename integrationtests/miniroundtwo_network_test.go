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
				Mutate: leaderBoundFaultMutator(
					t,
					committees.miniRoundTwo,
					scenario.Delivery.ClassificationVoteFaults,
					mutateClassificationVoteMessage,
				),
			},
		},
		testscommon.OrderedBroadcastRule{
			MessageType: data.AnswerEvidenceConsensusMessage,
			Mutate: broadcastFaultMutator(
				t,
				committees.miniRoundTwo,
				scenario.Delivery.AnswerEvidenceFaults,
				mutateAnswerEvidenceMessage,
			),
		},
		testscommon.OrderedBroadcastRule{
			MessageType: data.AnswerClassificationCertificateConsensusMessage,
			Mutate: broadcastFaultMutator(
				t,
				committees.miniRoundTwo,
				scenario.Delivery.ClassificationCertificateFaults,
				mutateClassificationCertificateMessage,
			),
		},
	)
}

func leaderBoundFaultMutator(
	t *testing.T,
	committee []string,
	faults []scenarioTransportFault,
	mutate func(data.ConsensusMessage, scenarioTransportFault) data.ConsensusMessage,
) testscommon.OrderedMessageMutator {
	t.Helper()
	if len(faults) == 0 {
		return nil
	}

	faultsBySenderID := make(map[string]scenarioTransportFault, len(faults))
	for _, fault := range faults {
		senderID := scenarioIDsForRoles(t, committee, []string{fault.SenderRole})[0]
		faultsBySenderID[senderID] = fault
	}

	return func(senderID string, message data.ConsensusMessage) (data.ConsensusMessage, bool) {
		fault, exists := faultsBySenderID[senderID]
		if !exists {
			return message, true
		}
		if fault.Type == "drop" {
			return data.ConsensusMessage{}, false
		}
		return mutate(message, fault), true
	}
}

func broadcastFaultMutator(
	t *testing.T,
	committee []string,
	faults []scenarioTransportFault,
	mutate func(*data.ConsensusMessage, scenarioTransportFault),
) testscommon.OrderedBroadcastMutator {
	t.Helper()
	if len(faults) == 0 {
		return nil
	}

	faultsBySenderID := make(map[string]scenarioTransportFault, len(faults))
	for _, fault := range faults {
		senderID := scenarioIDsForRoles(t, committee, []string{fault.SenderRole})[0]
		faultsBySenderID[senderID] = fault
	}

	return func(senderID string, message *data.ConsensusMessage) {
		fault, exists := faultsBySenderID[senderID]
		if !exists {
			return
		}
		mutate(message, fault)
	}
}

func mutateClassificationVoteMessage(
	message data.ConsensusMessage,
	fault scenarioTransportFault,
) data.ConsensusMessage {
	if fault.Type != "invalid_signature" || message.AnswerClassificationVote == nil {
		return message
	}

	mutated := message
	vote := *message.AnswerClassificationVote
	vote.Signature = append([]byte(nil), vote.Signature...)
	if len(vote.Signature) == 0 {
		vote.Signature = []byte("invalid-signature")
	} else {
		vote.Signature[0] ^= 0xff
	}
	mutated.AnswerClassificationVote = &vote
	return mutated
}

func mutateAnswerEvidenceMessage(message *data.ConsensusMessage, fault scenarioTransportFault) {
	if fault.Type != "mutate_evidence_hash" || message.AnswerEvidence == nil {
		return
	}

	message.AnswerEvidence.CanonicalBlockHash = append([]byte(nil), message.AnswerEvidence.CanonicalBlockHash...)
	if len(message.AnswerEvidence.CanonicalBlockHash) == 0 {
		message.AnswerEvidence.CanonicalBlockHash = []byte("invalid-canonical-block")
	} else {
		message.AnswerEvidence.CanonicalBlockHash[0] ^= 0xff
	}
}

func mutateClassificationCertificateMessage(message *data.ConsensusMessage, fault scenarioTransportFault) {
	if message.AnswerClassificationCertificate == nil {
		return
	}

	switch fault.Type {
	case "omit_certificate_vote":
		if len(message.AnswerClassificationCertificate.Votes) > 0 {
			message.AnswerClassificationCertificate.Votes = append(
				[]data.AnswerClassificationVote(nil),
				message.AnswerClassificationCertificate.Votes[:len(message.AnswerClassificationCertificate.Votes)-1]...,
			)
		}
	case "reorder_certificate_votes":
		if len(message.AnswerClassificationCertificate.Votes) > 1 {
			message.AnswerClassificationCertificate.Votes = append(
				[]data.AnswerClassificationVote(nil),
				message.AnswerClassificationCertificate.Votes...,
			)
			message.AnswerClassificationCertificate.Votes[0], message.AnswerClassificationCertificate.Votes[1] =
				message.AnswerClassificationCertificate.Votes[1], message.AnswerClassificationCertificate.Votes[0]
		}
	}
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
