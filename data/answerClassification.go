package data

import "bytes"

// AnswerCategory is the category assigned to a candidate answer. Each
// validator's judge selects one during the classification step.
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

// AnswerCandidateID uniquely identifies one answer occurrence. AnswerHash
// commits to the text, while ProducerID keeps identical answers from different
// validators separate. It is built from verified evidence before judging.
type AnswerCandidateID struct {
	ProducerID string
	TxHash     []byte
	AnswerHash []byte
}

// CompareAnswerCandidateIDs defines the canonical order used by validators
// while hashing, classifying, and finalizing candidates.
func CompareAnswerCandidateIDs(left, right AnswerCandidateID) int {
	if comparison := bytes.Compare(left.TxHash, right.TxHash); comparison != 0 {
		return comparison
	}
	if left.ProducerID < right.ProducerID {
		return -1
	}
	if left.ProducerID > right.ProducerID {
		return 1
	}

	return bytes.Compare(left.AnswerHash, right.AnswerHash)
}

// AnswerClassificationAssignment assigns exactly one category to one candidate.
// It is produced by each validator's judge.
type AnswerClassificationAssignment struct {
	CandidateID AnswerCandidateID
	Category    AnswerCategory
}

// AnswerClassificationVote contains one judge's canonical assignments. The
// judge signs it and sends it to the leader during vote collection. VoteHash and
// Signature are excluded when computing VoteHash.
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

// AnswerCategoryCounts contains classification support for one candidate. The
// leader derives it and validators recompute it when verifying the certificate.
type AnswerCategoryCounts struct {
	CandidateID   AnswerCandidateID
	Correct       uint64
	Hallucination uint64
	Malicious     uint64
	Wrong         uint64
}

// CanonicalAnswerGroups contains candidates assigned to each final category.
// It is derived after vote aggregation; mini-round three consumes Correct.
// Every group uses canonical candidate order.
type CanonicalAnswerGroups struct {
	Correct       []AnswerCandidateID
	Hallucination []AnswerCandidateID
	Malicious     []AnswerCandidateID
	Wrong         []AnswerCandidateID
}

// TransactionAnswerStatus defines whether a transaction can advance. It is set
// during mini-round-two finalization and consumed by mini-round three.
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

// TransactionAnswerClassification contains counts, groups, and status for one
// transaction. The leader derives it and validators recompute it before storing
// the finalized mini-round-two result.
type TransactionAnswerClassification struct {
	TxHash []byte
	Counts []AnswerCategoryCounts
	Groups CanonicalAnswerGroups
	Status TransactionAnswerStatus
}

// AnswerClassificationCertificate contains signed judge votes and their derived
// transaction results. The leader broadcasts it and validators verify it before
// finalizing mini-round two.
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
