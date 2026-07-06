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

// CanonicalizeAnswerClassifications returns a copy sorted by candidate ID.
func CanonicalizeAnswerClassifications(classifications []data.AnswerClassificationPerCandidate) []data.AnswerClassificationPerCandidate {
	ordered := append([]data.AnswerClassificationPerCandidate(nil), classifications...)
	sort.Slice(ordered, func(left, right int) bool {
		return data.CompareAnswerCandidateIDs(ordered[left].CandidateID, ordered[right].CandidateID) < 0
	})

	return ordered
}

// ValidateAnswerClassifications verifies exact candidate coverage, valid
// categories, uniqueness, and canonical ordering.
func ValidateAnswerClassifications(
	expectedCandidates []data.AnswerCandidateID,
	classifications []data.AnswerClassificationPerCandidate,
) error {
	expected, err := buildCandidateSet(expectedCandidates)
	if err != nil {
		return err
	}

	seen, err := validateClassificationSet(expected, classifications)
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

func validateClassificationSet(
	expected map[answerCandidateKey]struct{},
	classifications []data.AnswerClassificationPerCandidate,
) (map[answerCandidateKey]struct{}, error) {
	seen := make(map[answerCandidateKey]struct{}, len(classifications))
	for index := range classifications {
		key, err := validateClassification(expected, seen, classifications, index)
		if err != nil {
			return nil, err
		}

		seen[key] = struct{}{}
	}

	return seen, nil
}

func validateClassification(
	expected map[answerCandidateKey]struct{},
	seen map[answerCandidateKey]struct{},
	classifications []data.AnswerClassificationPerCandidate,
	index int,
) (answerCandidateKey, error) {
	classification := classifications[index]
	if err := validateCandidateID(classification.CandidateID); err != nil {
		return answerCandidateKey{}, err
	}
	if !classification.Category.IsValid() {
		return answerCandidateKey{}, ErrInvalidAnswerCategory
	}

	key := candidateKey(classification.CandidateID)
	if _, exists := seen[key]; exists {
		return answerCandidateKey{}, ErrDuplicatedAnswerCandidate
	}
	if _, exists := expected[key]; !exists {
		return answerCandidateKey{}, ErrUnknownAnswerCandidate
	}

	if index > 0 && data.CompareAnswerCandidateIDs(classifications[index-1].CandidateID, classification.CandidateID) >= 0 {
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

// ClassificationQuorum returns the number of matching classifications required
// by the initial protocol policy. The subtraction form is equivalent to
// floor(2*n/3)+1, but it cannot overflow when committeeSize is near MaxUint64.
func ClassificationQuorum(committeeSize uint64) uint64 {
	if committeeSize == 0 {
		return 0
	}

	return committeeSize - (committeeSize-1)/3
}

// AggregateClassificationVotes deterministically derives per-transaction
// counts, groups, and eligibility from judge votes. expectedCandidates comes
// from the verified answer-evidence certificate; votes are not allowed to define
// or reduce that candidate set.
func AggregateClassificationVotes(
	expectedCandidates []data.AnswerCandidateID,
	votes []data.AnswerClassificationVote,
	committeeSize uint64,
) ([]data.TransactionAnswerClassification, error) {
	// Aggregation is meaningful only after the judge certificate reaches quorum.
	// More votes than committee members indicates a malformed certificate.
	quorum := ClassificationQuorum(committeeSize)
	if quorum == 0 {
		return nil, ErrInvalidClassificationCommitteeSize
	}
	if uint64(len(votes)) < quorum || uint64(len(votes)) > committeeSize {
		return nil, ErrInvalidClassificationVoteCount
	}

	// Candidate input order must not affect counts, groups, or finalized output.
	candidates := CanonicalizeAnswerCandidateIDs(expectedCandidates)
	if len(candidates) == 0 {
		return nil, ErrMissingAnswerCandidate
	}

	if _, err := buildCandidateSet(candidates); err != nil {
		return nil, err
	}

	// Validate the complete vote set before mutating any counters. This keeps the
	// aggregator all-or-nothing when one judge submits malformed data.
	orderedVotes, err := validateAggregationVotes(candidates, votes)
	if err != nil {
		return nil, err
	}

	// Counts are keyed by the full candidate identity, not by answer text. Equal
	// text from different producers therefore remains independent evidence.
	counts := initializeCategoryCounts(candidates)

	// Count for each candidate (txHash, answerHash, producerID) how many judges classified it in a specific category.
	countClassificationVotes(counts, orderedVotes)

	// TxHash -> [correct answers, malicious answers, wrong answers, hallucinations]
	// TxHash -> (eligible for mini-round two or not eligible)
	return buildTransactionClassifications(candidates, counts, quorum), nil
}

func validateAggregationVotes(
	candidates []data.AnswerCandidateID,
	votes []data.AnswerClassificationVote,
) ([]data.AnswerClassificationVote, error) {
	// Arrival order is not consensus-visible. Judge ID defines certificate order
	// and also makes duplicate judge votes detectable.
	orderedVotes := CanonicalizeClassificationVotes(votes)
	if err := ValidateCanonicalClassificationVotes(orderedVotes); err != nil {
		return nil, err
	}
	if len(orderedVotes) == 0 {
		return nil, ErrInvalidClassificationVoteCount
	}

	// Every vote must classify the same evidence with the same protocol prompt.
	// ModelMetadata may differ because validators can use different local agents.
	firstVote := orderedVotes[0]
	for _, vote := range orderedVotes {
		if err := validateClassificationVoteContext(vote); err != nil {
			return nil, err
		}

		if !SameVoteContext(firstVote, vote) {
			return nil, ErrClassificationVoteContextMismatch
		}

		if err := ValidateAnswerClassifications(candidates, vote.AnswerClassifications); err != nil {
			return nil, err
		}
	}

	return orderedVotes, nil
}

func validateClassificationVoteContext(vote data.AnswerClassificationVote) error {
	if len(vote.CanonicalBlockHash) == 0 ||
		len(vote.AnswerEvidenceHash) == 0 ||
		vote.PromptVersion == "" ||
		len(vote.PromptHash) == 0 {
		return ErrInvalidClassificationVote
	}

	return nil
}

// SameVoteContext reports whether two votes commit to the same round, answer
// evidence, canonical block, and judge-prompt version. Judge-specific fields
// are deliberately excluded.
func SameVoteContext(left, right data.AnswerClassificationVote) bool {
	// Judge ID, model metadata, hash, and signature are intentionally omitted:
	// they identify or authenticate an individual vote rather than shared input.
	return left.Epoch == right.Epoch &&
		left.Round == right.Round &&
		left.MiniRound == right.MiniRound &&
		bytes.Equal(left.CanonicalBlockHash, right.CanonicalBlockHash) &&
		bytes.Equal(left.AnswerEvidenceHash, right.AnswerEvidenceHash) &&
		left.PromptVersion == right.PromptVersion &&
		bytes.Equal(left.PromptHash, right.PromptHash)
}

func initializeCategoryCounts(
	candidates []data.AnswerCandidateID,
) map[answerCandidateKey]*data.AnswerCategoryCounts {
	// Pre-create every counter so a candidate remains in the result even when a
	// particular category receives zero votes.
	counts := make(map[answerCandidateKey]*data.AnswerCategoryCounts, len(candidates))
	for _, candidate := range candidates {
		counts[candidateKey(candidate)] = &data.AnswerCategoryCounts{CandidateID: candidate}
	}

	return counts
}

func countClassificationVotes(
	counts map[answerCandidateKey]*data.AnswerCategoryCounts,
	votes []data.AnswerClassificationVote,
) {
	// Classification coverage and category validity were checked before this pass, so
	// every map lookup is guaranteed to resolve to an initialized counter.
	for _, vote := range votes {
		for _, classification := range vote.AnswerClassifications {
			incrementCategoryCount(counts[candidateKey(classification.CandidateID)], classification.Category)
		}
	}
}

func incrementCategoryCount(counts *data.AnswerCategoryCounts, category data.AnswerCategory) {
	switch category {
	case data.AnswerCategoryCorrect:
		counts.Correct++
	case data.AnswerCategoryHallucination:
		counts.Hallucination++
	case data.AnswerCategoryMalicious:
		counts.Malicious++
	case data.AnswerCategoryWrong:
		counts.Wrong++
	}
}

func buildTransactionClassifications(
	candidates []data.AnswerCandidateID,
	counts map[answerCandidateKey]*data.AnswerCategoryCounts,
	quorum uint64,
) []data.TransactionAnswerClassification {
	// candidates is globally ordered by txHash, producer ID, and answer hash.
	// Go maps are useful above for direct counter lookup, but map iteration order
	// is nondeterministic and cannot define consensus-visible transaction order.
	// Grouping adjacent candidates in this already sorted slice creates canonical
	// transactions, counts, and groups without building a map and sorting its keys
	// again. A new transaction starts only when the candidate txHash changes.
	transactions := make([]data.TransactionAnswerClassification, 0)
	for _, candidate := range candidates {
		if len(transactions) == 0 || !bytes.Equal(transactions[len(transactions)-1].TxHash, candidate.TxHash) {
			transactions = append(transactions, data.TransactionAnswerClassification{TxHash: candidate.TxHash})
		}

		transaction := &transactions[len(transactions)-1]
		candidateCounts := *counts[candidateKey(candidate)]
		transaction.Counts = append(transaction.Counts, candidateCounts)
		addCandidateToGroup(&transaction.Groups, candidateCounts, quorum)
	}

	for index := range transactions {
		transactions[index].Status = classificationStatus(transactions[index].Groups.Correct, quorum)
	}

	return transactions
}

func addCandidateToGroup(
	groups *data.CanonicalAnswerGroups,
	counts data.AnswerCategoryCounts,
	quorum uint64,
) {
	// Correct is special: it requires the full protocol quorum. A mere plurality
	// of Correct votes cannot place an answer in the canonical Correct group.
	if counts.Correct >= quorum {
		groups.Correct = append(groups.Correct, counts.CandidateID)
		return
	}

	// Once Correct misses quorum, only the other three categories participate in
	// plurality selection, even when Correct has the largest raw count.
	switch highestNonCorrectCategory(counts) {
	case data.AnswerCategoryHallucination:
		groups.Hallucination = append(groups.Hallucination, counts.CandidateID)
	case data.AnswerCategoryMalicious:
		groups.Malicious = append(groups.Malicious, counts.CandidateID)
	case data.AnswerCategoryWrong:
		groups.Wrong = append(groups.Wrong, counts.CandidateID)
	}
}

func highestNonCorrectCategory(counts data.AnswerCategoryCounts) data.AnswerCategory {
	// Initializing with Wrong implements the conservative tie order:
	// Wrong > Hallucination > Malicious. Later categories replace the current one
	// only when they have strictly greater support.
	category := data.AnswerCategoryWrong
	highestCount := counts.Wrong
	if counts.Hallucination > highestCount {
		category = data.AnswerCategoryHallucination
		highestCount = counts.Hallucination
	}
	if counts.Malicious > highestCount {
		category = data.AnswerCategoryMalicious
	}

	return category
}

func classificationStatus(
	correctCandidates []data.AnswerCandidateID,
	quorum uint64,
) data.TransactionAnswerStatus {
	// Eligibility counts distinct producers rather than candidate records. This
	// prevents multiple answers attributed to one producer from satisfying 2f+1.
	producers := make(map[string]struct{}, len(correctCandidates))
	for _, candidate := range correctCandidates {
		producers[candidate.ProducerID] = struct{}{}
	}

	if uint64(len(producers)) >= quorum {
		return data.TransactionAnswerStatusReadyForMiniRoundThree
	}

	return data.TransactionAnswerStatusInsufficientCorrectAnswers
}
