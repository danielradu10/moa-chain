package data

type VoteType string

const (
	VoteTypeCommit VoteType = "COMMIT"
)

// ConsensusMessage defines the standardized consensus message which will be shared between rounds.
type ConsensusMessage struct {
	ConsensusMessageType ConsensusMessageType

	// Mini-Round One
	ProposedBlockMessage *ProposedBlockMessage
	AggregatedVotes      *AggregatedVotes
	BlockVote            *BlockVote

	// Mini-Round Two
	ExecutedPrompts                 *AnswersBlockMessage
	AnswerEvidence                  *AggregatedExecutionResultsMessage
	AnswerClassificationVote        *AnswerClassificationVote
	AnswerClassificationCertificate *AnswerClassificationCertificate

	// Mini-Round Three
	ProposedSynthesis        *ProposedSynthesisMessage
	SynthesisVote            *SynthesisVote
	AggregatedSynthesisVotes *AggregatedSynthesisVotes
}

type ConsensusMessageType int

const (
	ProposedBlockConsensusMessage   ConsensusMessageType = iota // Mini-Round One
	AggregatedVotesConsensusMessage                             // Mini-Round One
	BlockVoteConsensusMessage                                   // Mini-Round One

	ExecutedPromptsMessage                          // Mini-Round Two
	AnswerEvidenceConsensusMessage                  // Mini-Round Two
	AnswerClassificationVoteConsensusMessage        // Mini-Round Two
	AnswerClassificationCertificateConsensusMessage // Mini-Round Two

	ProposedSynthesisConsensusMessage        // Mini-Round Three
	SynthesisVoteConsensusMessage            // Mini-Round Three
	AggregatedSynthesisVotesConsensusMessage // Mini-Round Three
)

// MINI-ROUND ONE
// ##################################

// ProposedBlockMessage defines the message propagated by a leader and received by all validators.
type ProposedBlockMessage struct {
	// RoundKey information
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Sender related information
	SenderID string

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

	// BlockSignature related information.
	BlockHash      []byte
	BlockSignature []byte

	// SubdomainsSignature related information
	Subdomains          Subdomains
	SubdomainsSignature []byte
}

// AggregatedVotes defines the data structure containing the votes aggregated by the leader, which will later be verified by all validators.
type AggregatedVotes struct {
	// RoundKey information.
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Leader related information.
	SenderID string

	// BlockVote related information.
	BlockHash  []byte
	Signers    [][]byte
	Signatures [][]byte

	// Subdomains related information.
	Subdomains           []Subdomains
	SubdomainsSignatures [][]byte
}

// ##################################

// MINI-ROUND TWO

// ValidatorVote contains the validator public key and its vote.
// It is used only when extracting the state of the votes in order to keep a simplified, minimized version of the BlockVote.
type ValidatorVote struct {
	ValidatorID         string
	BlockSignature      []byte
	Subdomains          Subdomains
	SubdomainsSignature []byte
}

// AnswersBlockMessage defines the message propagated by the experts in the mini-round two after the prompt execution.
type AnswersBlockMessage struct {
	// RoundKey information.
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Sender related information.
	SenderID string

	// Prompts Answers
	Answers AnswersTxMessage

	// CanonicalBlock related information.
	CanonicalBlockHash []byte // header hash of the finalized block in mini-round one.

	// Block related information.
	BlockHash      []byte // this is the hash of this block.
	BlockSignature []byte
}

type AnswersTxMessage map[string]TransactionResult

// AggregatedExecutionResultsMessage defines the execution result certificate collected by the leader in mini-round two.
// Signers, block hashes, signatures, and answers are aligned by index.
type AggregatedExecutionResultsMessage struct {
	// RoundKey information.
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	// Leader related information.
	SenderID string

	// CanonicalBlock related information.
	CanonicalBlockHash []byte // header hash of the finalized block in mini-round one.

	// Execution result signatures related information.
	Signers         []string
	BlockHashes     [][]byte
	BlockSignatures [][]byte

	// Answers related information.
	Answers []AnswersTxMessage
}
