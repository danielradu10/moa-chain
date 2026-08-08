package data

// SynthesizedAnswer holds the leader's synthesized response for one transaction.
type SynthesizedAnswer struct {
	TxHash     []byte
	Answer     string
	AnswerHash []byte // SHA-256 of Answer
}

// ProposedSynthesisMessage is broadcast by the leader to all validators after
// synthesizing answers from the mini-round-two correct groups.
// SynthesizedAnswers are sorted by transaction hash for deterministic hashing.
type ProposedSynthesisMessage struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	SenderID string

	// CanonicalBlockHash is the mini-round-one block header hash, carried
	// forward from the mini-round-two finalized block.
	CanonicalBlockHash []byte

	// AnswerEvidenceHash binds this proposal to the exact mini-round-two answer
	// evidence the leader used to extract correct answers.
	AnswerEvidenceHash []byte

	// SynthesizedAnswers contains one entry per eligible transaction, sorted
	// by transaction hash.
	SynthesizedAnswers []SynthesizedAnswer

	// SynthesisBlockHash is the deterministic hash of the canonical proposal
	// payload (all fields above). It is the value signers commit to.
	SynthesisBlockHash []byte
	Signature          []byte
}

// SynthesisVote is the binary approval cast by a committee member after
// verifying and evaluating the leader's proposed synthesis. Casting a vote
// means full approval of every synthesized answer in the proposal; a validator
// that rejects any transaction simply does not send a vote.
type SynthesisVote struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	VoterID string

	// SynthesisBlockHash commits this vote to the exact proposal being approved.
	SynthesisBlockHash []byte

	VoteHash  []byte
	Signature []byte
}

// AggregatedSynthesisVotes is the quorum certificate built by the leader from
// accepted SynthesisVotes. Signers, VoteHashes, and Signatures are parallel
// slices aligned by index and sorted by validator ID.
type AggregatedSynthesisVotes struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	SenderID string

	SynthesisBlockHash []byte

	Signers    []string
	VoteHashes [][]byte
	Signatures [][]byte
}

// FinalAnswerStatus describes the mini-round-three outcome for one transaction.
type FinalAnswerStatus string

const (
	// FinalAnswerStatusSynthesized means the transaction received a synthesized
	// answer that was approved by the committee quorum.
	FinalAnswerStatusSynthesized FinalAnswerStatus = "SYNTHESIZED"

	// FinalAnswerStatusSkipped means the transaction was not eligible for
	// synthesis because its mini-round-two status was InsufficientCorrectAnswers
	// or NonRelatedTransaction.
	FinalAnswerStatusSkipped FinalAnswerStatus = "SKIPPED"
)

// FinalAnswer is the on-chain output for one transaction after mini-round
// three. Answer is empty when Status is FinalAnswerStatusSkipped.
// The slice on BlockOnChain is sorted by transaction hash.
type FinalAnswer struct {
	TxHash []byte
	Answer string
	Status FinalAnswerStatus
}
