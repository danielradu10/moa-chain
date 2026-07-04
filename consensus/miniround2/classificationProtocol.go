package miniround2

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"sort"

	"moa-chain/data"
)

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

func compareCandidateIDs(left, right data.AnswerCandidateID) int {
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

func validateCandidateID(candidate data.AnswerCandidateID) error {
	if candidate.ProducerID == "" || len(candidate.TxHash) == 0 || len(candidate.AnswerHash) != sha256.Size {
		return fmt.Errorf("%w: producer=%q txHashLength=%d answerHashLength=%d",
			ErrInvalidAnswerCandidate,
			candidate.ProducerID,
			len(candidate.TxHash),
			len(candidate.AnswerHash),
		)
	}

	return nil
}

// CanonicalizeAnswerCandidateIDs returns a sorted copy. Candidates are ordered
// by transaction hash, producer ID, and answer hash.
func CanonicalizeAnswerCandidateIDs(candidates []data.AnswerCandidateID) []data.AnswerCandidateID {
	ordered := append([]data.AnswerCandidateID(nil), candidates...)
	sort.Slice(ordered, func(left, right int) bool {
		return compareCandidateIDs(ordered[left], ordered[right]) < 0
	})

	return ordered
}

// CanonicalizeClassificationAssignments returns a copy sorted by candidate ID.
func CanonicalizeClassificationAssignments(assignments []data.AnswerClassificationAssignment) []data.AnswerClassificationAssignment {
	ordered := append([]data.AnswerClassificationAssignment(nil), assignments...)
	sort.Slice(ordered, func(left, right int) bool {
		return compareCandidateIDs(ordered[left].CandidateID, ordered[right].CandidateID) < 0
	})

	return ordered
}

// ValidateClassificationAssignments verifies exact candidate coverage, valid
// categories, uniqueness, and canonical ordering.
func ValidateClassificationAssignments(
	expectedCandidates []data.AnswerCandidateID,
	assignments []data.AnswerClassificationAssignment,
) error {
	expected := make(map[answerCandidateKey]struct{}, len(expectedCandidates))
	for _, candidate := range expectedCandidates {
		if err := validateCandidateID(candidate); err != nil {
			return err
		}

		key := candidateKey(candidate)
		if _, exists := expected[key]; exists {
			return fmt.Errorf("%w: producer=%q", ErrDuplicatedAnswerCandidate, candidate.ProducerID)
		}
		expected[key] = struct{}{}
	}

	seen := make(map[answerCandidateKey]struct{}, len(assignments))
	for index, assignment := range assignments {
		if err := validateCandidateID(assignment.CandidateID); err != nil {
			return err
		}
		if !assignment.Category.IsValid() {
			return fmt.Errorf("%w: %q", ErrInvalidAnswerCategory, assignment.Category)
		}

		key := candidateKey(assignment.CandidateID)
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: producer=%q", ErrDuplicatedAnswerCandidate, assignment.CandidateID.ProducerID)
		}
		if _, exists := expected[key]; !exists {
			return fmt.Errorf("%w: producer=%q", ErrUnknownAnswerCandidate, assignment.CandidateID.ProducerID)
		}
		seen[key] = struct{}{}

		if index > 0 && compareCandidateIDs(assignments[index-1].CandidateID, assignment.CandidateID) >= 0 {
			return ErrNonCanonicalClassification
		}
	}

	if len(seen) != len(expected) {
		return fmt.Errorf("%w: expected=%d actual=%d", ErrMissingAnswerCandidate, len(expected), len(seen))
	}

	return nil
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
			return fmt.Errorf("%w: empty judge ID", ErrInvalidClassificationVote)
		}
		if _, exists := seen[vote.JudgeID]; exists {
			return fmt.Errorf("%w: %q", ErrDuplicatedClassificationJudge, vote.JudgeID)
		}
		seen[vote.JudgeID] = struct{}{}

		if index > 0 && votes[index-1].JudgeID >= vote.JudgeID {
			return ErrNonCanonicalClassification
		}
	}

	return nil
}

// CanonicalizeTransactionClassifications returns a copy sorted by transaction hash.
func CanonicalizeTransactionClassifications(
	transactions []data.TransactionAnswerClassification,
) []data.TransactionAnswerClassification {
	ordered := append([]data.TransactionAnswerClassification(nil), transactions...)
	sort.Slice(ordered, func(left, right int) bool {
		return bytes.Compare(ordered[left].TxHash, ordered[right].TxHash) < 0
	})

	return ordered
}

// ValidateCanonicalTransactionClassifications verifies transaction ordering and
// the canonical candidate ordering of counts and every group.
func ValidateCanonicalTransactionClassifications(transactions []data.TransactionAnswerClassification) error {
	for index, transaction := range transactions {
		if len(transaction.TxHash) == 0 || !transaction.Status.IsValid() {
			return ErrNonCanonicalClassification
		}
		if index > 0 && bytes.Compare(transactions[index-1].TxHash, transaction.TxHash) >= 0 {
			return ErrNonCanonicalClassification
		}

		countCandidates := make([]data.AnswerCandidateID, 0, len(transaction.Counts))
		for _, count := range transaction.Counts {
			if !bytes.Equal(count.CandidateID.TxHash, transaction.TxHash) {
				return fmt.Errorf("%w: candidate transaction hash mismatch", ErrInvalidAnswerCandidate)
			}
			countCandidates = append(countCandidates, count.CandidateID)
		}
		if err := validateCanonicalCandidateIDs(countCandidates); err != nil {
			return err
		}
		expectedGroupCandidates := make(map[answerCandidateKey]struct{}, len(countCandidates))
		for _, candidate := range countCandidates {
			expectedGroupCandidates[candidateKey(candidate)] = struct{}{}
		}

		groups := [][]data.AnswerCandidateID{
			transaction.Groups.Correct,
			transaction.Groups.Hallucination,
			transaction.Groups.Malicious,
			transaction.Groups.Wrong,
		}
		groupCandidates := make(map[answerCandidateKey]struct{}, len(countCandidates))
		for _, group := range groups {
			if err := validateCanonicalCandidateIDs(group); err != nil {
				return err
			}
			for _, candidate := range group {
				if !bytes.Equal(candidate.TxHash, transaction.TxHash) {
					return fmt.Errorf("%w: group candidate transaction hash mismatch", ErrInvalidAnswerCandidate)
				}
				key := candidateKey(candidate)
				if _, exists := expectedGroupCandidates[key]; !exists {
					return fmt.Errorf("%w: producer=%q", ErrUnknownAnswerCandidate, candidate.ProducerID)
				}
				if _, exists := groupCandidates[key]; exists {
					return fmt.Errorf("%w: producer=%q", ErrDuplicatedAnswerCandidate, candidate.ProducerID)
				}
				groupCandidates[key] = struct{}{}
			}
		}

		if len(groupCandidates) != len(countCandidates) {
			return fmt.Errorf("%w: group coverage expected=%d actual=%d",
				ErrMissingAnswerCandidate,
				len(countCandidates),
				len(groupCandidates),
			)
		}
	}

	return nil
}

func validateCanonicalCandidateIDs(candidates []data.AnswerCandidateID) error {
	for index, candidate := range candidates {
		if err := validateCandidateID(candidate); err != nil {
			return err
		}
		if index > 0 && compareCandidateIDs(candidates[index-1], candidate) >= 0 {
			return ErrNonCanonicalClassification
		}
	}

	return nil
}
