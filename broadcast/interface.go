package broadcast

import (
	"moa-chain/data"
)

// Broadcaster defines what a Broadcaster should do.
type Broadcaster interface {
	SendVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error
	BroadcastProposedBlock(blockMessage *data.ConsensusMessage, myID string, receivers []string) error
	BroadcastAggregatedVotes(aggregatedVotesMessage *data.ConsensusMessage, myID string, receivers []string) error
}

// PeerRegistry defines what a peer registry should do.
type PeerRegistry interface {
	Register(validatorID string, channel chan<- data.RoundEvent) error
	Unregister(validatorID string)
	GetChannel(validatorID string) (chan<- data.RoundEvent, error)
}
