package classification

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/hashing"
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
		AnswerHash: hashing.ComputeAnswerHash(answer),
	}
}

func TestClassificationQuorum(t *testing.T) {
	t.Parallel()

	tests := []struct {
		committeeSize uint64
		expected      uint64
	}{
		{committeeSize: 0, expected: 0},
		{committeeSize: 1, expected: 1},
		{committeeSize: 3, expected: 3},
		{committeeSize: 4, expected: 3},
		{committeeSize: 7, expected: 5},
		{committeeSize: 10, expected: 7},
		{committeeSize: ^uint64(0), expected: ^uint64(0) - (^uint64(0)-1)/3},
	}

	for _, test := range tests {
		require.Equal(t, test.expected, ClassificationQuorum(test.committeeSize))
	}
}

func TestAggregateClassificationVotes(t *testing.T) {
	t.Parallel()

	candidates := aggregationCandidates()
	votes := aggregationVotes(candidates)

	result, err := AggregateClassificationVotes(candidates, votes, 4)
	require.NoError(t, err)

	reversedVotes := []data.AnswerClassificationVote{votes[2], votes[1], votes[0]}
	reversedResult, err := AggregateClassificationVotes(candidates, reversedVotes, 4)
	require.NoError(t, err)
	require.Equal(t, result, reversedResult)

	require.Len(t, result, 2)
	require.Equal(t, []byte("tx-a"), result[0].TxHash)
	require.Equal(t, data.TransactionAnswerStatusReadyForMiniRoundThree, result[0].Status)
	require.Len(t, result[0].Groups.Correct, 3)
	require.Len(t, result[0].Groups.Hallucination, 1)
	require.Empty(t, result[0].Groups.Malicious)
	require.Empty(t, result[0].Groups.Wrong)

	require.Equal(t, []byte("tx-b"), result[1].TxHash)
	require.Equal(t, data.TransactionAnswerStatusInsufficientCorrectAnswers, result[1].Status)
	require.Len(t, result[1].Groups.Correct, 2)
	require.Empty(t, result[1].Groups.Hallucination)
	require.Empty(t, result[1].Groups.Malicious)
	require.Len(t, result[1].Groups.Wrong, 2)
	require.Equal(t, uint64(2), result[1].Counts[3].Correct)
	require.Equal(t, uint64(1), result[1].Counts[3].Wrong)

	require.NoError(t, ValidateCanonicalTransactionClassifications(result))
}

func TestHighestNonCorrectCategoryTieBreaking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		counts   data.AnswerCategoryCounts
		expected data.AnswerCategory
	}{
		{
			counts: data.AnswerCategoryCounts{
				Wrong: 1, Hallucination: 1, Malicious: 1,
			},
			expected: data.AnswerCategoryWrong,
		},
		{
			counts: data.AnswerCategoryCounts{
				Hallucination: 1, Malicious: 1,
			},
			expected: data.AnswerCategoryHallucination,
		},
		{
			counts: data.AnswerCategoryCounts{
				Hallucination: 1, Malicious: 2,
			},
			expected: data.AnswerCategoryMalicious,
		},
	}

	for _, test := range tests {
		require.Equal(t, test.expected, highestNonCorrectCategory(test.counts))
	}
}

func TestAggregateClassificationVotesRequiresIndependentCorrectProducers(t *testing.T) {
	t.Parallel()

	candidates := CanonicalizeAnswerCandidateIDs([]data.AnswerCandidateID{
		classificationCandidate("producer-a", "tx-a", "first answer"),
		classificationCandidate("producer-a", "tx-a", "second answer"),
		classificationCandidate("producer-b", "tx-a", "third answer"),
	})
	categories := []data.AnswerCategory{
		data.AnswerCategoryCorrect,
		data.AnswerCategoryCorrect,
		data.AnswerCategoryCorrect,
	}
	votes := []data.AnswerClassificationVote{
		aggregationVote("judge-a", candidates, categories),
		aggregationVote("judge-b", candidates, categories),
		aggregationVote("judge-c", candidates, categories),
	}

	result, err := AggregateClassificationVotes(candidates, votes, 4)
	require.NoError(t, err)

	require.Len(t, result[0].Groups.Correct, 3)
	require.Equal(t, data.TransactionAnswerStatusInsufficientCorrectAnswers, result[0].Status)
}

func TestAggregateClassificationVotesRejectsMalformedInput(t *testing.T) {
	t.Parallel()

	candidates := aggregationCandidates()
	validVotes := aggregationVotes(candidates)

	tests := []struct {
		name          string
		candidates    []data.AnswerCandidateID
		votes         []data.AnswerClassificationVote
		committeeSize uint64
		targetError   error
	}{
		{
			name:          "zero committee",
			candidates:    candidates,
			votes:         validVotes,
			committeeSize: 0,
			targetError:   ErrInvalidClassificationCommitteeSize,
		},
		{
			name:          "insufficient votes",
			candidates:    candidates,
			votes:         validVotes[:2],
			committeeSize: 4,
			targetError:   ErrInvalidClassificationVoteCount,
		},
		{
			name:          "empty candidates",
			votes:         validVotes,
			committeeSize: 4,
			targetError:   ErrMissingAnswerCandidate,
		},
		{
			name:          "duplicate expected candidate",
			candidates:    append(append([]data.AnswerCandidateID(nil), candidates...), candidates[0]),
			votes:         validVotes,
			committeeSize: 4,
			targetError:   ErrDuplicatedAnswerCandidate,
		},
		{
			name:       "duplicate judge",
			candidates: candidates,
			votes: []data.AnswerClassificationVote{
				validVotes[0], validVotes[0], validVotes[2],
			},
			committeeSize: 4,
			targetError:   ErrDuplicatedClassificationJudge,
		},
		{
			name:       "missing assignment",
			candidates: candidates,
			votes: mutateAggregationVotes(validVotes, func(vote *data.AnswerClassificationVote) {
				vote.Assignments = vote.Assignments[:len(vote.Assignments)-1]
			}),
			committeeSize: 4,
			targetError:   ErrMissingAnswerCandidate,
		},
		{
			name:       "non-canonical assignments",
			candidates: candidates,
			votes: mutateAggregationVotes(validVotes, func(vote *data.AnswerClassificationVote) {
				vote.Assignments[0], vote.Assignments[1] = vote.Assignments[1], vote.Assignments[0]
			}),
			committeeSize: 4,
			targetError:   ErrNonCanonicalClassification,
		},
		{
			name:       "invalid category",
			candidates: candidates,
			votes: mutateAggregationVotes(validVotes, func(vote *data.AnswerClassificationVote) {
				vote.Assignments[0].Category = data.AnswerCategory("UNKNOWN")
			}),
			committeeSize: 4,
			targetError:   ErrInvalidAnswerCategory,
		},
		{
			name:       "different evidence",
			candidates: candidates,
			votes: mutateAggregationVotes(validVotes, func(vote *data.AnswerClassificationVote) {
				vote.AnswerEvidenceHash = []byte("different-evidence")
			}),
			committeeSize: 4,
			targetError:   ErrClassificationVoteContextMismatch,
		},
		{
			name:       "missing prompt version",
			candidates: candidates,
			votes: mutateAggregationVotes(validVotes, func(vote *data.AnswerClassificationVote) {
				vote.PromptVersion = ""
			}),
			committeeSize: 4,
			targetError:   ErrInvalidClassificationVote,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			result, err := AggregateClassificationVotes(test.candidates, test.votes, test.committeeSize)

			require.Nil(t, result)
			require.ErrorIs(t, err, test.targetError)
		})
	}
}

func aggregationCandidates() []data.AnswerCandidateID {
	candidates := []data.AnswerCandidateID{
		classificationCandidate("producer-a", "tx-a", "tx-a answer a"),
		classificationCandidate("producer-b", "tx-a", "tx-a answer b"),
		classificationCandidate("producer-c", "tx-a", "tx-a answer c"),
		classificationCandidate("producer-d", "tx-a", "tx-a answer d"),
		classificationCandidate("producer-a", "tx-b", "tx-b answer a"),
		classificationCandidate("producer-b", "tx-b", "tx-b answer b"),
		classificationCandidate("producer-c", "tx-b", "tx-b answer c"),
		classificationCandidate("producer-d", "tx-b", "tx-b answer d"),
	}

	return CanonicalizeAnswerCandidateIDs(candidates)
}

func aggregationVotes(candidates []data.AnswerCandidateID) []data.AnswerClassificationVote {
	return []data.AnswerClassificationVote{
		aggregationVote("judge-a", candidates, []data.AnswerCategory{
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryHallucination,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryWrong,
			data.AnswerCategoryCorrect,
		}),
		aggregationVote("judge-b", candidates, []data.AnswerCategory{
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryHallucination,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryHallucination,
			data.AnswerCategoryCorrect,
		}),
		aggregationVote("judge-c", candidates, []data.AnswerCategory{
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryMalicious,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryCorrect,
			data.AnswerCategoryMalicious,
			data.AnswerCategoryWrong,
		}),
	}
}

func aggregationVote(
	judgeID string,
	candidates []data.AnswerCandidateID,
	categories []data.AnswerCategory,
) data.AnswerClassificationVote {
	assignments := make([]data.AnswerClassificationAssignment, len(candidates))
	for index, candidate := range candidates {
		assignments[index] = data.AnswerClassificationAssignment{
			CandidateID: candidate,
			Category:    categories[index],
		}
	}

	return data.AnswerClassificationVote{
		Epoch:              1,
		Round:              2,
		MiniRound:          1,
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		JudgeID:            judgeID,
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		Assignments:        assignments,
	}
}

func mutateAggregationVotes(
	votes []data.AnswerClassificationVote,
	mutate func(*data.AnswerClassificationVote),
) []data.AnswerClassificationVote {
	result := append([]data.AnswerClassificationVote(nil), votes...)
	result[0].Assignments = append([]data.AnswerClassificationAssignment(nil), votes[0].Assignments...)
	mutate(&result[0])

	return result
}
