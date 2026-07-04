package miniround2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestComputeAnswerEvidenceHash(t *testing.T) {
	t.Parallel()

	first := classificationAnswerEvidence()
	second := classificationAnswerEvidence()
	second.Signers[0], second.Signers[1] = second.Signers[1], second.Signers[0]
	second.BlockHashes[0], second.BlockHashes[1] = second.BlockHashes[1], second.BlockHashes[0]
	second.BlockSignatures[0], second.BlockSignatures[1] = second.BlockSignatures[1], second.BlockSignatures[0]
	second.Answers[0], second.Answers[1] = second.Answers[1], second.Answers[0]
	second.Answers[0] = data.AnswersTxMessage{
		"tx-b": second.Answers[0]["tx-b"],
		"tx-a": second.Answers[0]["tx-a"],
	}

	firstHash, err := ComputeAnswerEvidenceHash(first)
	require.NoError(t, err)
	secondHash, err := ComputeAnswerEvidenceHash(second)
	require.NoError(t, err)

	require.Equal(t, firstHash, secondHash)
	require.Len(t, firstHash, 32)

	changed := classificationAnswerEvidence()
	changed.Answers[0]["tx-a"] = data.TransactionResult{
		TxHash:            []byte("tx-a"),
		Answer:            "changed answer",
		ActualConsumption: 2,
	}
	changedHash, err := ComputeAnswerEvidenceHash(changed)
	require.NoError(t, err)
	require.NotEqual(t, firstHash, changedHash)
}

func TestComputeAnswerEvidenceHashRejectsMalformedEvidence(t *testing.T) {
	t.Parallel()

	_, err := ComputeAnswerEvidenceHash(nil)
	require.ErrorIs(t, err, ErrInvalidAnswerEvidence)

	misaligned := classificationAnswerEvidence()
	misaligned.BlockHashes = misaligned.BlockHashes[:1]
	_, err = ComputeAnswerEvidenceHash(misaligned)
	require.ErrorIs(t, err, ErrInvalidAnswerEvidence)

	duplicatedSigner := classificationAnswerEvidence()
	duplicatedSigner.Signers[1] = duplicatedSigner.Signers[0]
	_, err = ComputeAnswerEvidenceHash(duplicatedSigner)
	require.ErrorIs(t, err, ErrInvalidAnswerEvidence)
}

func TestComputeClassificationVoteHash(t *testing.T) {
	t.Parallel()

	vote := classificationVote()
	reversed := classificationVote()
	reversed.Assignments[0], reversed.Assignments[1] = reversed.Assignments[1], reversed.Assignments[0]
	reversed.VoteHash = []byte("ignored vote hash")
	reversed.Signature = []byte("ignored signature")

	firstHash, err := ComputeClassificationVoteHash(vote)
	require.NoError(t, err)
	secondHash, err := ComputeClassificationVoteHash(reversed)
	require.NoError(t, err)

	require.Equal(t, firstHash, secondHash)
	require.Len(t, firstHash, 32)

	changed := classificationVote()
	changed.Assignments[0].Category = data.AnswerCategoryWrong
	changedHash, err := ComputeClassificationVoteHash(changed)
	require.NoError(t, err)
	require.NotEqual(t, firstHash, changedHash)
}

func TestComputeClassificationVoteHashRejectsMalformedVote(t *testing.T) {
	t.Parallel()

	_, err := ComputeClassificationVoteHash(nil)
	require.ErrorIs(t, err, ErrInvalidClassificationVote)

	missingAssignment := classificationVote()
	missingAssignment.Assignments = nil
	_, err = ComputeClassificationVoteHash(missingAssignment)
	require.ErrorIs(t, err, ErrMissingAnswerCandidate)

	duplicate := classificationVote()
	duplicate.Assignments[1].CandidateID = duplicate.Assignments[0].CandidateID
	_, err = ComputeClassificationVoteHash(duplicate)
	require.ErrorIs(t, err, ErrDuplicatedAnswerCandidate)

	invalidCategory := classificationVote()
	invalidCategory.Assignments[0].Category = data.AnswerCategory("UNKNOWN")
	_, err = ComputeClassificationVoteHash(invalidCategory)
	require.ErrorIs(t, err, ErrInvalidAnswerCategory)
}

func classificationAnswerEvidence() *data.AggregatedExecutionResultsMessage {
	return &data.AggregatedExecutionResultsMessage{
		Epoch:              1,
		Round:              2,
		MiniRound:          1,
		SenderID:           "leader",
		CanonicalBlockHash: []byte("canonical-block"),
		Signers:            []string{"validator-b", "validator-a"},
		BlockHashes:        [][]byte{[]byte("block-b"), []byte("block-a")},
		BlockSignatures:    [][]byte{[]byte("signature-b"), []byte("signature-a")},
		Answers: []data.AnswersTxMessage{
			{
				"tx-a": {TxHash: []byte("tx-a"), Answer: "answer a from b", ActualConsumption: 2},
				"tx-b": {TxHash: []byte("tx-b"), Answer: "answer b from b", ActualConsumption: 3},
			},
			{
				"tx-a": {TxHash: []byte("tx-a"), Answer: "answer a from a", ActualConsumption: 4},
				"tx-b": {TxHash: []byte("tx-b"), Answer: "answer b from a", ActualConsumption: 5},
			},
		},
	}
}

func classificationVote() *data.AnswerClassificationVote {
	return &data.AnswerClassificationVote{
		Epoch:              1,
		Round:              2,
		MiniRound:          1,
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		JudgeID:            "validator-a",
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		ModelMetadata:      "test-model",
		Assignments: []data.AnswerClassificationAssignment{
			{
				CandidateID: classificationCandidate("validator-a", "tx-a", "answer a"),
				Category:    data.AnswerCategoryCorrect,
			},
			{
				CandidateID: classificationCandidate("validator-b", "tx-a", "answer b"),
				Category:    data.AnswerCategoryHallucination,
			},
		},
	}
}
