package broadcast

import (
	"log/slog"

	"moa-chain/data"
	"moa-chain/logging"
)

type broadcaster struct {
	registry PeerRegistry
	logger   *slog.Logger
}

// NewBroadcaster creates a new broadcaster
func NewBroadcaster(peerRegistry PeerRegistry, loggers ...*slog.Logger) *broadcaster {
	return &broadcaster{
		registry: peerRegistry,
		logger:   logging.FromOptional(loggers...),
	}
}

// SendVoteToLeader sends vote message to leader.
func (b *broadcaster) SendVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error {
	leaderChannel, err := b.registry.GetChannel(leaderID)
	if err != nil {
		b.logger.Error("failed to get leader channel", "leaderID", leaderID, "error", err)
		return err
	}

	if leaderChannel == nil {
		b.logger.Error("leader channel is nil", "leaderID", leaderID)
		return ErrNilChannel
	}

	if voteMessage == nil {
		b.logger.Error("cannot send nil vote message to leader", "leaderID", leaderID)
		return ErrNilConsensusMessage
	}

	b.logger.Info("broadcast.SendVoteToLeader sending vote to leader", "leaderID", leaderID, "messageType", voteMessage.ConsensusMessageType)
	leaderChannel <- data.RoundEvent{
		Type:    data.ConsensusMessageEvent,
		Message: *voteMessage,
	}
	return nil
}

// SendAnswerClassificationVoteToLeader sends a judge vote to the mini-round-two leader.
func (b *broadcaster) SendAnswerClassificationVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error {
	return b.SendVoteToLeader(voteMessage, leaderID)
}

func (b *broadcaster) BroadcastProposedBlock(blockMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(blockMessage, myID, receivers)
}

func (b *broadcaster) BroadcastAggregatedVotes(aggregatedVotesMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(aggregatedVotesMessage, myID, receivers)
}

// BroadcastAnswerEvidence sends the leader's verified producer-answer
// certificate to validators so they can independently verify and judge it.
func (b *broadcaster) BroadcastAnswerEvidence(answerEvidenceMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(answerEvidenceMessage, myID, receivers)
}

// BroadcastAnswerClassificationCertificate sends the classification evidence and derived groups to validators.
func (b *broadcaster) BroadcastAnswerClassificationCertificate(certificateMessage *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(certificateMessage, myID, receivers)
}

// BroadcastProposedSynthesis sends the leader's synthesis proposal to all validators.
func (b *broadcaster) BroadcastProposedSynthesis(msg *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(msg, myID, receivers)
}

// SendSynthesisVoteToLeader sends a binary synthesis approval vote to the MR3 leader.
func (b *broadcaster) SendSynthesisVoteToLeader(msg *data.ConsensusMessage, leaderID string) error {
	return b.SendVoteToLeader(msg, leaderID)
}

// BroadcastAggregatedSynthesisVotes sends the quorum certificate to all validators.
func (b *broadcaster) BroadcastAggregatedSynthesisVotes(msg *data.ConsensusMessage, myID string, receivers []string) error {
	return b.broadcast(msg, myID, receivers)
}

func (b *broadcaster) broadcast(message *data.ConsensusMessage, myID string, receivers []string) error {
	if message == nil {
		b.logger.Error("cannot broadcast nil consensus message", "senderID", myID)
		return ErrNilConsensusMessage
	}

	b.logger.Info(
		"broadcast.broadcast broadcasting consensus message",
		"senderID", myID,
		"messageType", message.ConsensusMessageType,
		"numReceivers", len(receivers),
	)

	for _, receiver := range receivers {
		if myID == receiver {
			continue
		}

		channel, err := b.registry.GetChannel(receiver)
		if err != nil {
			b.logger.Error("failed to get receiver channel", "senderID", myID, "receiverID", receiver, "error", err)
			return err
		}

		if channel == nil {
			b.logger.Error("receiver channel is nil", "senderID", myID, "receiverID", receiver)
			return ErrNilChannel
		}

		b.logger.Debug("broadcasting message to receiver", "senderID", myID, "receiverID", receiver, "messageType", message.ConsensusMessageType)
		channel <- data.RoundEvent{
			Type:    data.ConsensusMessageEvent,
			Message: *message,
		}
	}

	return nil
}
