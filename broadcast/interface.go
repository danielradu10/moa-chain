package broadcast

import (
	"moa-chain/data"
)

// Broadcaster defines what a Broadcaster should do.
type Broadcaster interface {
	SendVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error
	SendAnswerClassificationVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error
	BroadcastProposedBlock(blockMessage *data.ConsensusMessage, myID string, receivers []string) error
	BroadcastAggregatedVotes(aggregatedVotesMessage *data.ConsensusMessage, myID string, receivers []string) error
	BroadcastAnswerEvidence(answerEvidenceMessage *data.ConsensusMessage, myID string, receivers []string) error
	BroadcastAnswerClassificationCertificate(certificateMessage *data.ConsensusMessage, myID string, receivers []string) error

	// Mini-Round Three
	BroadcastProposedSynthesis(msg *data.ConsensusMessage, myID string, receivers []string) error
	SendSynthesisVoteToLeader(msg *data.ConsensusMessage, leaderID string) error
	BroadcastAggregatedSynthesisVotes(msg *data.ConsensusMessage, myID string, receivers []string) error
}

// PeerRegistry defines what a peer registry should do.
type PeerRegistry interface {
	Register(validatorID string, channel chan<- data.RoundEvent) error
	Unregister(validatorID string)
	GetChannel(validatorID string) (chan<- data.RoundEvent, error)
}
