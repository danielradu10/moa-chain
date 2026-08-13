package integrationtests

import (
	"testing"

	"moa-chain/data"
	"moa-chain/testscommon"
)

func newMiniRoundThreeScenarioNetwork(
	t *testing.T,
	inboxes []chan data.RoundEvent,
	committees mr3ScenarioCommittees,
	scenario miniRoundThreeScenario,
) *testscommon.OrderedBroadcasterNetworkStub {
	t.Helper()

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
				SenderOrder: scenarioIDsForRoles(
					t,
					committees.miniRoundTwo,
					scenario.Delivery.JudgeOrder,
				),
				StartIndex: 1,
				Mutate: leaderBoundFaultMutator(
					t,
					committees.miniRoundTwo,
					scenario.Delivery.ClassificationVoteFaults,
					mutateClassificationVoteMessage,
				),
			},
			{
				MessageType: data.SynthesisVoteConsensusMessage,
				SenderOrder: scenarioIDsForRoles(
					t,
					committees.miniRoundThree,
					scenario.Delivery.SynthesizerVoteOrder,
				),
				// The leader does not send a synthesis vote through the broadcaster;
				// delivery starts at member-1 (index 1).
				StartIndex: 1,
				Mutate: mr3SynthesisVoteFaultMutator(
					t,
					committees.miniRoundThree,
					scenario.Delivery.SynthesisVoteFaults,
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

func mr3SynthesisVoteFaultMutator(
	t *testing.T,
	committee []string,
	faults []scenarioSynthesisVoteFault,
) testscommon.OrderedMessageMutator {
	t.Helper()
	if len(faults) == 0 {
		return nil
	}

	faultsBySenderID := make(map[string]scenarioSynthesisVoteFault, len(faults))
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
		return mutateSynthesisVoteMessage(message, fault), true
	}
}

func mutateSynthesisVoteMessage(
	message data.ConsensusMessage,
	fault scenarioSynthesisVoteFault,
) data.ConsensusMessage {
	if message.SynthesisVote == nil {
		return message
	}

	mutated := message
	vote := *message.SynthesisVote

	switch fault.Type {
	case "invalid_signature":
		vote.Signature = append([]byte(nil), vote.Signature...)
		if len(vote.Signature) == 0 {
			vote.Signature = []byte("invalid-signature")
		} else {
			vote.Signature[0] ^= 0xff
		}
	case "wrong_synthesis_hash":
		vote.SynthesisBlockHash = append([]byte(nil), vote.SynthesisBlockHash...)
		if len(vote.SynthesisBlockHash) == 0 {
			vote.SynthesisBlockHash = []byte("invalid-synthesis-hash")
		} else {
			vote.SynthesisBlockHash[0] ^= 0xff
		}
	}

	mutated.SynthesisVote = &vote
	return mutated
}
