package broadcast

import (
	"moa-chain/data"
)

type broadcaster struct {
	registry PeerRegistry
}

// NewBroadcaster creates a new broadcaster
func NewBroadcaster(peerRegistry PeerRegistry) *broadcaster {
	return &broadcaster{
		registry: peerRegistry,
	}
}

// SendVoteToLeader sends vote message to leader.
func (b *broadcaster) SendVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error {
	leaderChannel, err := b.registry.GetChannel(leaderID)
	if err != nil {
		return err
	}

	if leaderChannel == nil {
		return ErrNilChannel
	}

	if voteMessage == nil {
		return ErrNilConsensusMessage
	}

	leaderChannel <- data.RoundEvent{
		Type:    data.ConsensusMessageEvent,
		Message: *voteMessage,
	}
	return nil
}

func (b *broadcaster) BroadcastProposedBlock(blockMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(blockMessage, myID, receivers)
}

func (b *broadcaster) BroadcastAggregatedVotes(aggregatedVotesMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(aggregatedVotesMessage, myID, receivers)
}

func (b *broadcaster) broadcast(message *data.ConsensusMessage, myID string, receivers []string) error {
	if message == nil {
		return ErrNilConsensusMessage
	}

	for _, receiver := range receivers {
		if myID == receiver {
			continue
		}

		channel, err := b.registry.GetChannel(receiver)
		if err != nil {
			return err
		}

		if channel == nil {
			return ErrNilChannel
		}

		channel <- data.RoundEvent{
			Type:    data.ConsensusMessageEvent,
			Message: *message,
		}
	}

	return nil
}
