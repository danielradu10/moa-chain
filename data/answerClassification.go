package data

// AnswerCategory is the category assigned by a validator's answer judge.
type AnswerCategory string

const (
	AnswerCategoryCorrect       AnswerCategory = "CORRECT"
	AnswerCategoryHallucination AnswerCategory = "HALLUCINATION"
	AnswerCategoryMalicious     AnswerCategory = "MALICIOUS"
	AnswerCategoryWrong         AnswerCategory = "WRONG"
)

// IsValid reports whether the category is defined by the protocol.
func (category AnswerCategory) IsValid() bool {
	switch category {
	case AnswerCategoryCorrect,
		AnswerCategoryHallucination,
		AnswerCategoryMalicious,
		AnswerCategoryWrong:
		return true
	default:
		return false
	}
}

// AnswerCandidateID uniquely identifies one answer occurrence. AnswerHash commits
// to the answer text, while ProducerID keeps identical answers from different
// validators as separate candidates.
type AnswerCandidateID struct {
	ProducerID string
	TxHash     []byte
	AnswerHash []byte
}

// AnswerClassificationAssignment assigns exactly one category to one candidate.
type AnswerClassificationAssignment struct {
	CandidateID AnswerCandidateID
	Category    AnswerCategory
}

// AnswerClassificationVote is the signed classification produced by one judge.
// Assignments must be in canonical candidate order. VoteHash and Signature are
// excluded when computing VoteHash.
type AnswerClassificationVote struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	CanonicalBlockHash []byte
	AnswerEvidenceHash []byte

	JudgeID       string
	PromptVersion string
	PromptHash    []byte
	ModelMetadata string

	Assignments []AnswerClassificationAssignment

	VoteHash  []byte
	Signature []byte
}

// AnswerCategoryCounts contains classification support for one candidate.
type AnswerCategoryCounts struct {
	CandidateID   AnswerCandidateID
	Correct       uint64
	Hallucination uint64
	Malicious     uint64
	Wrong         uint64
}

// CanonicalAnswerGroups contains candidate IDs assigned to each final group.
// Every slice must be in canonical candidate order.
type CanonicalAnswerGroups struct {
	Correct       []AnswerCandidateID
	Hallucination []AnswerCandidateID
	Malicious     []AnswerCandidateID
	Wrong         []AnswerCandidateID
}

// TransactionAnswerStatus defines whether a transaction can advance to
// mini-round three.
type TransactionAnswerStatus string

const (
	TransactionAnswerStatusReadyForMiniRoundThree     TransactionAnswerStatus = "READY_FOR_MINI_ROUND_THREE"
	TransactionAnswerStatusInsufficientCorrectAnswers TransactionAnswerStatus = "INSUFFICIENT_CORRECT_ANSWERS"
)

// IsValid reports whether the transaction status is defined by the protocol.
func (status TransactionAnswerStatus) IsValid() bool {
	switch status {
	case TransactionAnswerStatusReadyForMiniRoundThree,
		TransactionAnswerStatusInsufficientCorrectAnswers:
		return true
	default:
		return false
	}
}

// TransactionAnswerClassification is the deterministic result for one
// transaction. Counts and group members must use canonical candidate order.
type TransactionAnswerClassification struct {
	TxHash []byte
	Counts []AnswerCategoryCounts
	Groups CanonicalAnswerGroups
	Status TransactionAnswerStatus
}

// AnswerClassificationCertificate contains signed judge votes and the result
// derived from them. Votes must be ordered by judge ID and transactions by hash.
type AnswerClassificationCertificate struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64

	SenderID           string
	CanonicalBlockHash []byte
	AnswerEvidenceHash []byte
	PromptVersion      string
	PromptHash         []byte

	Votes        []AnswerClassificationVote
	Transactions []TransactionAnswerClassification
}
