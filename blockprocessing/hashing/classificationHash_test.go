package hashing

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
	reversed.AnswerClassifications[0], reversed.AnswerClassifications[1] = reversed.AnswerClassifications[1], reversed.AnswerClassifications[0]
	reversed.VoteHash = []byte("ignored vote hash")
	reversed.Signature = []byte("ignored signature")

	firstHash, err := ComputeClassificationVoteHash(vote)
	require.NoError(t, err)
	secondHash, err := ComputeClassificationVoteHash(reversed)
	require.NoError(t, err)

	require.Equal(t, firstHash, secondHash)
	require.Len(t, firstHash, 32)

	changed := classificationVote()
	changed.AnswerClassifications[0].Category = data.AnswerCategoryWrong
	changedHash, err := ComputeClassificationVoteHash(changed)
	require.NoError(t, err)
	require.NotEqual(t, firstHash, changedHash)
}

func TestComputeClassificationVoteHashRejectsMalformedVote(t *testing.T) {
	t.Parallel()

	_, err := ComputeClassificationVoteHash(nil)
	require.ErrorIs(t, err, ErrInvalidClassificationVote)

	missingClassifications := classificationVote()
	missingClassifications.AnswerClassifications = nil
	_, err = ComputeClassificationVoteHash(missingClassifications)
	require.ErrorIs(t, err, ErrInvalidClassificationVote)

	duplicate := classificationVote()
	duplicate.AnswerClassifications[1].CandidateID = duplicate.AnswerClassifications[0].CandidateID
	_, err = ComputeClassificationVoteHash(duplicate)
	require.ErrorIs(t, err, ErrInvalidClassificationVote)

	invalidCategory := classificationVote()
	invalidCategory.AnswerClassifications[0].Category = data.AnswerCategory("UNKNOWN")
	_, err = ComputeClassificationVoteHash(invalidCategory)
	require.ErrorIs(t, err, ErrInvalidClassificationVote)
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
		AnswerClassifications: []data.AnswerClassificationPerCandidate{
			{
				CandidateID: classificationHashCandidate("validator-a", "tx-a", "answer a"),
				Category:    data.AnswerCategoryCorrect,
			},
			{
				CandidateID: classificationHashCandidate("validator-b", "tx-a", "answer b"),
				Category:    data.AnswerCategoryHallucination,
			},
		},
	}
}

func classificationHashCandidate(producerID, txHash, answer string) data.AnswerCandidateID {
	return data.AnswerCandidateID{
		ProducerID: producerID,
		TxHash:     []byte(txHash),
		AnswerHash: ComputeAnswerHash(answer),
	}
}
