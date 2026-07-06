// Package classification implements the deterministic mini-round-two
// classification protocol and the local, non-deterministic LLM judge adapter.
// It has no networking or round-state responsibilities.
package classification

import (
	"bytes"
	"crypto/sha256"
	"sort"

	"moa-chain/data"
)

// answerCandidateKey is the comparable form of AnswerCandidateID used for map
// membership checks. Converting byte slices to strings preserves their bytes.
type answerCandidateKey struct {
	producerID string
	txHash     string
	answerHash string
}

func candidateKey(candidate data.AnswerCandidateID) answerCandidateKey {
	return answerCandidateKey{
		producerID: candidate.ProducerID,
		txHash:     string(candidate.TxHash),
		answerHash: string(candidate.AnswerHash),
	}
}

func validateCandidateID(candidate data.AnswerCandidateID) error {
	if candidate.ProducerID == "" || len(candidate.TxHash) == 0 || len(candidate.AnswerHash) != sha256.Size {
		return ErrInvalidAnswerCandidate
	}

	return nil
}

// CanonicalizeAnswerCandidateIDs returns a sorted copy. Candidates are ordered
// by transaction hash, producer ID, and answer hash.
func CanonicalizeAnswerCandidateIDs(candidates []data.AnswerCandidateID) []data.AnswerCandidateID {
	ordered := append([]data.AnswerCandidateID(nil), candidates...)
	sort.Slice(ordered, func(left, right int) bool {
		return data.CompareAnswerCandidateIDs(ordered[left], ordered[right]) < 0
	})

	return ordered
}

// CanonicalizeClassificationAssignments returns a copy sorted by candidate ID.
func CanonicalizeClassificationAssignments(assignments []data.AnswerClassificationAssignment) []data.AnswerClassificationAssignment {
	ordered := append([]data.AnswerClassificationAssignment(nil), assignments...)
	sort.Slice(ordered, func(left, right int) bool {
		return data.CompareAnswerCandidateIDs(ordered[left].CandidateID, ordered[right].CandidateID) < 0
	})

	return ordered
}

// ValidateClassificationAssignments verifies exact candidate coverage, valid
// categories, uniqueness, and canonical ordering.
func ValidateClassificationAssignments(
	expectedCandidates []data.AnswerCandidateID,
	assignments []data.AnswerClassificationAssignment,
) error {
	expected, err := buildCandidateSet(expectedCandidates)
	if err != nil {
		return err
	}

	seen, err := validateAssignmentSet(expected, assignments)
	if err != nil {
		return err
	}
	if len(seen) != len(expected) {
		return ErrMissingAnswerCandidate
	}

	return nil
}

func buildCandidateSet(candidates []data.AnswerCandidateID) (map[answerCandidateKey]struct{}, error) {
	result := make(map[answerCandidateKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		if err := validateCandidateID(candidate); err != nil {
			return nil, err
		}

		key := candidateKey(candidate)
		if _, exists := result[key]; exists {
			return nil, ErrDuplicatedAnswerCandidate
		}

		result[key] = struct{}{}
	}

	return result, nil
}

func validateAssignmentSet(
	expected map[answerCandidateKey]struct{},
	assignments []data.AnswerClassificationAssignment,
) (map[answerCandidateKey]struct{}, error) {
	seen := make(map[answerCandidateKey]struct{}, len(assignments))
	for index := range assignments {
		key, err := validateAssignment(expected, seen, assignments, index)
		if err != nil {
			return nil, err
		}

		seen[key] = struct{}{}
	}

	return seen, nil
}

func validateAssignment(
	expected map[answerCandidateKey]struct{},
	seen map[answerCandidateKey]struct{},
	assignments []data.AnswerClassificationAssignment,
	index int,
) (answerCandidateKey, error) {
	assignment := assignments[index]
	if err := validateCandidateID(assignment.CandidateID); err != nil {
		return answerCandidateKey{}, err
	}
	if !assignment.Category.IsValid() {
		return answerCandidateKey{}, ErrInvalidAnswerCategory
	}

	key := candidateKey(assignment.CandidateID)
	if _, exists := seen[key]; exists {
		return answerCandidateKey{}, ErrDuplicatedAnswerCandidate
	}
	if _, exists := expected[key]; !exists {
		return answerCandidateKey{}, ErrUnknownAnswerCandidate
	}

	if index > 0 && data.CompareAnswerCandidateIDs(assignments[index-1].CandidateID, assignment.CandidateID) >= 0 {
		return answerCandidateKey{}, ErrNonCanonicalClassification
	}

	return key, nil
}

// CanonicalizeClassificationVotes returns a copy sorted by judge ID.
func CanonicalizeClassificationVotes(votes []data.AnswerClassificationVote) []data.AnswerClassificationVote {
	ordered := append([]data.AnswerClassificationVote(nil), votes...)
	sort.Slice(ordered, func(left, right int) bool {
		return ordered[left].JudgeID < ordered[right].JudgeID
	})

	return ordered
}

// ValidateCanonicalClassificationVotes verifies non-empty, unique judge IDs and
// canonical judge ordering.
func ValidateCanonicalClassificationVotes(votes []data.AnswerClassificationVote) error {
	seen := make(map[string]struct{}, len(votes))
	for index, vote := range votes {
		if vote.JudgeID == "" {
			return ErrInvalidClassificationVote
		}
		if _, exists := seen[vote.JudgeID]; exists {
			return ErrDuplicatedClassificationJudge
		}
		seen[vote.JudgeID] = struct{}{}

		if index > 0 && votes[index-1].JudgeID >= vote.JudgeID {
			return ErrNonCanonicalClassification
		}
	}

	return nil
}

// ValidateCanonicalTransactionClassifications verifies transaction ordering and
// the canonical candidate ordering of counts and every group.
func ValidateCanonicalTransactionClassifications(transactions []data.TransactionAnswerClassification) error {
	for index, transaction := range transactions {
		if err := validateTransactionPosition(transactions, index); err != nil {
			return err
		}
		if err := validateTransactionClassification(transaction); err != nil {
			return err
		}
	}

	return nil
}

func validateTransactionPosition(transactions []data.TransactionAnswerClassification, index int) error {
	transaction := transactions[index]
	if len(transaction.TxHash) == 0 || !transaction.Status.IsValid() {
		return ErrNonCanonicalClassification
	}
	if index > 0 && bytes.Compare(transactions[index-1].TxHash, transaction.TxHash) >= 0 {
		return ErrNonCanonicalClassification
	}

	return nil
}

func validateTransactionClassification(transaction data.TransactionAnswerClassification) error {
	countCandidates, err := extractCountCandidates(transaction)
	if err != nil {
		return err
	}
	if err = validateCanonicalCandidateIDs(countCandidates); err != nil {
		return err
	}

	expectedCandidates := candidateSet(countCandidates)
	groupCandidates, err := validateAnswerGroups(transaction.TxHash, expectedCandidates, transaction.Groups)
	if err != nil {
		return err
	}
	if len(groupCandidates) != len(expectedCandidates) {
		return ErrMissingAnswerCandidate
	}

	return nil
}

func extractCountCandidates(transaction data.TransactionAnswerClassification) ([]data.AnswerCandidateID, error) {
	candidates := make([]data.AnswerCandidateID, 0, len(transaction.Counts))
	for _, count := range transaction.Counts {
		if !bytes.Equal(count.CandidateID.TxHash, transaction.TxHash) {
			return nil, ErrInvalidAnswerCandidate
		}
		candidates = append(candidates, count.CandidateID)
	}

	return candidates, nil
}

func candidateSet(candidates []data.AnswerCandidateID) map[answerCandidateKey]struct{} {
	result := make(map[answerCandidateKey]struct{}, len(candidates))
	for _, candidate := range candidates {
		result[candidateKey(candidate)] = struct{}{}
	}

	return result
}

func validateAnswerGroups(
	txHash []byte,
	expected map[answerCandidateKey]struct{},
	groups data.CanonicalAnswerGroups,
) (map[answerCandidateKey]struct{}, error) {
	seen := make(map[answerCandidateKey]struct{}, len(expected))
	for _, group := range answerGroups(groups) {
		if err := validateAnswerGroup(txHash, expected, seen, group); err != nil {
			return nil, err
		}
	}

	return seen, nil
}

func answerGroups(groups data.CanonicalAnswerGroups) [][]data.AnswerCandidateID {
	return [][]data.AnswerCandidateID{
		groups.Correct,
		groups.Hallucination,
		groups.Malicious,
		groups.Wrong,
	}
}

func validateAnswerGroup(
	txHash []byte,
	expected map[answerCandidateKey]struct{},
	seen map[answerCandidateKey]struct{},
	group []data.AnswerCandidateID,
) error {
	if err := validateCanonicalCandidateIDs(group); err != nil {
		return err
	}
	for _, candidate := range group {
		if !bytes.Equal(candidate.TxHash, txHash) {
			return ErrInvalidAnswerCandidate
		}
		key := candidateKey(candidate)
		if _, exists := expected[key]; !exists {
			return ErrUnknownAnswerCandidate
		}
		if _, exists := seen[key]; exists {
			return ErrDuplicatedAnswerCandidate
		}
		seen[key] = struct{}{}
	}

	return nil
}

func validateCanonicalCandidateIDs(candidates []data.AnswerCandidateID) error {
	for index, candidate := range candidates {
		if err := validateCandidateID(candidate); err != nil {
			return err
		}
		if index > 0 && data.CompareAnswerCandidateIDs(candidates[index-1], candidate) >= 0 {
			return ErrNonCanonicalClassification
		}
	}

	return nil
}
