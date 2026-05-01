package broadcast

import (
	"moa-chain/data"
)

// Broadcaster defines what a Broadcaster should do.
type Broadcaster interface {
	SendVoteToLeader(vote *data.BlockVote) error
	BroadcastProposedBlock(blockMessage *data.ProposedBlockMessage) error
	BroadcastAggregatedVotes(aggregatedVotes data.AggregatedVotes) error
}
