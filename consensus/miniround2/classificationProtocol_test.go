package miniround2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestCanonicalizeAnswerCandidateIDs(t *testing.T) {
	t.Parallel()

	candidates := []data.AnswerCandidateID{
		classificationCandidate("validator-b", "tx-b", "answer-3"),
		classificationCandidate("validator-b", "tx-a", "answer-2"),
		classificationCandidate("validator-a", "tx-a", "answer-1"),
	}

	ordered := CanonicalizeAnswerCandidateIDs(candidates)

	require.Equal(t, "validator-a", ordered[0].ProducerID)
	require.Equal(t, "validator-b", ordered[1].ProducerID)
	require.Equal(t, []byte("tx-b"), ordered[2].TxHash)
	require.Equal(t, "validator-b", candidates[0].ProducerID, "input must not be mutated")
}

func TestValidateClassificationAssignments(t *testing.T) {
	t.Parallel()

	first := classificationCandidate("validator-a", "tx-a", "same answer")
	second := classificationCandidate("validator-b", "tx-a", "same answer")
	expected := CanonicalizeAnswerCandidateIDs([]data.AnswerCandidateID{second, first})
	valid := []data.AnswerClassificationAssignment{
		{CandidateID: expected[0], Category: data.AnswerCategoryCorrect},
		{CandidateID: expected[1], Category: data.AnswerCategoryWrong},
	}

	require.Equal(t, first.AnswerHash, second.AnswerHash)
	require.NoError(t, ValidateClassificationAssignments(expected, valid),
		"identical text from different producers must remain separate")

	tests := []struct {
		name        string
		assignments []data.AnswerClassificationAssignment
		targetError error
	}{
		{
			name:        "missing candidate",
			assignments: valid[:1],
			targetError: ErrMissingAnswerCandidate,
		},
		{
			name: "duplicate candidate",
			assignments: []data.AnswerClassificationAssignment{
				valid[0],
				valid[0],
			},
			targetError: ErrDuplicatedAnswerCandidate,
		},
		{
			name: "unknown candidate",
			assignments: []data.AnswerClassificationAssignment{
				valid[0],
				{
					CandidateID: classificationCandidate("validator-c", "tx-a", "answer"),
					Category:    data.AnswerCategoryWrong,
				},
			},
			targetError: ErrUnknownAnswerCandidate,
		},
		{
			name: "invalid category",
			assignments: []data.AnswerClassificationAssignment{
				valid[0],
				{CandidateID: valid[1].CandidateID, Category: data.AnswerCategory("OTHER")},
			},
			targetError: ErrInvalidAnswerCategory,
		},
		{
			name: "non-canonical ordering",
			assignments: []data.AnswerClassificationAssignment{
				valid[1],
				valid[0],
			},
			targetError: ErrNonCanonicalClassification,
		},
		{
			name: "invalid candidate hash",
			assignments: []data.AnswerClassificationAssignment{
				valid[0],
				{
					CandidateID: data.AnswerCandidateID{
						ProducerID: valid[1].CandidateID.ProducerID,
						TxHash:     valid[1].CandidateID.TxHash,
						AnswerHash: []byte("not-sha-256"),
					},
					Category: data.AnswerCategoryWrong,
				},
			},
			targetError: ErrInvalidAnswerCandidate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			err := ValidateClassificationAssignments(expected, test.assignments)

			require.ErrorIs(t, err, test.targetError)
		})
	}
}

func TestCanonicalClassificationVoteOrdering(t *testing.T) {
	t.Parallel()

	votes := []data.AnswerClassificationVote{
		{JudgeID: "validator-c"},
		{JudgeID: "validator-a"},
		{JudgeID: "validator-b"},
	}

	ordered := CanonicalizeClassificationVotes(votes)

	require.Equal(t, []string{"validator-a", "validator-b", "validator-c"}, []string{
		ordered[0].JudgeID,
		ordered[1].JudgeID,
		ordered[2].JudgeID,
	})
	require.NoError(t, ValidateCanonicalClassificationVotes(ordered))
	require.ErrorIs(t,
		ValidateCanonicalClassificationVotes([]data.AnswerClassificationVote{
			{JudgeID: "validator-a"},
			{JudgeID: "validator-a"},
		}),
		ErrDuplicatedClassificationJudge,
	)
	require.ErrorIs(t,
		ValidateCanonicalClassificationVotes(votes),
		ErrNonCanonicalClassification,
	)
}

func TestValidateCanonicalTransactionClassifications(t *testing.T) {
	t.Parallel()

	txACandidate := classificationCandidate("validator-a", "tx-a", "answer-a")
	txBCandidate := classificationCandidate("validator-a", "tx-b", "answer-b")
	transactions := []data.TransactionAnswerClassification{
		{
			TxHash: []byte("tx-a"),
			Counts: []data.AnswerCategoryCounts{{CandidateID: txACandidate, Correct: 3}},
			Groups: data.CanonicalAnswerGroups{Correct: []data.AnswerCandidateID{txACandidate}},
			Status: data.TransactionAnswerStatusReadyForMiniRoundThree,
		},
		{
			TxHash: []byte("tx-b"),
			Counts: []data.AnswerCategoryCounts{{CandidateID: txBCandidate, Wrong: 3}},
			Groups: data.CanonicalAnswerGroups{Wrong: []data.AnswerCandidateID{txBCandidate}},
			Status: data.TransactionAnswerStatusInsufficientCorrectAnswers,
		},
	}

	require.NoError(t, ValidateCanonicalTransactionClassifications(transactions))

	reversed := []data.TransactionAnswerClassification{transactions[1], transactions[0]}
	require.ErrorIs(t, ValidateCanonicalTransactionClassifications(reversed), ErrNonCanonicalClassification)

	missingGroupMember := append([]data.TransactionAnswerClassification(nil), transactions...)
	missingGroupMember[0].Groups = data.CanonicalAnswerGroups{}
	require.ErrorIs(t,
		ValidateCanonicalTransactionClassifications(missingGroupMember),
		ErrMissingAnswerCandidate,
	)

	wrongTransactionCandidate := append([]data.TransactionAnswerClassification(nil), transactions...)
	wrongTransactionCandidate[0].Counts = []data.AnswerCategoryCounts{{CandidateID: txBCandidate}}
	require.ErrorIs(t,
		ValidateCanonicalTransactionClassifications(wrongTransactionCandidate),
		ErrInvalidAnswerCandidate,
	)
}

func TestAnswerCategoryAndTransactionStatusValidation(t *testing.T) {
	t.Parallel()

	require.True(t, data.AnswerCategoryCorrect.IsValid())
	require.True(t, data.AnswerCategoryHallucination.IsValid())
	require.True(t, data.AnswerCategoryMalicious.IsValid())
	require.True(t, data.AnswerCategoryWrong.IsValid())
	require.False(t, data.AnswerCategory("UNKNOWN").IsValid())

	require.True(t, data.TransactionAnswerStatusReadyForMiniRoundThree.IsValid())
	require.True(t, data.TransactionAnswerStatusInsufficientCorrectAnswers.IsValid())
	require.False(t, data.TransactionAnswerStatus("UNKNOWN").IsValid())
}

func classificationCandidate(producerID, txHash, answer string) data.AnswerCandidateID {
	return data.AnswerCandidateID{
		ProducerID: producerID,
		TxHash:     []byte(txHash),
		AnswerHash: ComputeAnswerHash(answer),
	}
}
