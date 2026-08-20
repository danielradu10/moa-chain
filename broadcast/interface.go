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

// TxBroadcaster delivers a raw transaction to all registered peers except the
// sender. Only the raw transaction is propagated — labels and answers are
// never shared across nodes.
//
// TODO(tx-readiness-gossip): after a node's TxPreprocessor finishes labeling
// and answering a transaction, it should broadcast a signed TxReady(txHash,
// nodeID) message to all peers via an additional BroadcastTxReady method.
// Proposers should collect TxReady signatures and only include a transaction
// in a proposed block once they hold signatures from ≥ quorum validators.
// This eliminates wasted proposal slots caused by timing variance in LLM
// inference: a fast node cannot force a block that slower validators cannot
// yet validate.
type TxBroadcaster interface {
	BroadcastTransaction(tx data.Transaction, senderID string)
}

// TxPeerRegistry maps node IDs to their transaction inbox channels.
type TxPeerRegistry interface {
	Register(nodeID string, inbox chan<- data.Transaction) error
	GetAll() map[string]chan<- data.Transaction
}
