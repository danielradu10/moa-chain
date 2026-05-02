package data

type VoteType string

const (
	VoteTypeCommit VoteType = "COMMIT"
)

// ProposedBlockMessage defines the message propagated by a leader and received by all validators.
type ProposedBlockMessage struct {
	// RoundKey information
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Block related information.
	Block *Block
}

// BlockVote defines the vote propagated by a validator and collected by the leader.
type BlockVote struct {
	// RoundKey information
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Vote related information.
	SignerID string // we need either the singerID, either directly its public key.
	VoteType VoteType

	// Signature related information.
	BlockHash []byte
	Signature []byte
}

// AggregatedVotes defines the data structure containing the votes aggregated by the leader, which will later be verified by all validators.
type AggregatedVotes struct {
	// RoundKey information.
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Vote related information.
	BlockHash  []byte
	Signers    [][]byte
	Signatures [][]byte
}

// ConsensusMessage defines the standardized consensus message which will be shared between rounds.
type ConsensusMessage struct {
	ConsensusMessageType ConsensusMessageType

	ProposedBlockMessage *ProposedBlockMessage
	AggregatedVotes      *AggregatedVotes
	BlockVote            *BlockVote
}

type ConsensusMessageType int

const (
	ProposedBlockConsensusMessage ConsensusMessageType = iota
	AggregatedVotesConsensusMessage
	BlockVoteConsensusMessage
)

// ValidatorVote contains the validator public key and its vote.
type ValidatorVote struct {
	ValidatorID string
	Signature   []byte
}
