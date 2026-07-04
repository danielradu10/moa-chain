package miniround2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/state"
)

func TestMiniRoundTwoHandler_HandleAnswerClassificationVote(t *testing.T) {
	t.Parallel()

	roundKey := createTestRoundKey()
	roundState := state.NewRoundState()
	handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{roundState: roundState})
	vote := plumbingClassificationVote(roundKey, "judge-a")

	err := handler.HandleAnswerClassificationVote(roundKey, vote)
	require.NoError(t, err)
	votes, err := roundState.GetAnswerClassificationVotes(roundKey)
	require.NoError(t, err)
	require.Equal(t, []*data.AnswerClassificationVote{vote}, votes)

	err = handler.HandleAnswerClassificationVote(roundKey, vote)
	require.ErrorIs(t, err, state.ErrAnswerClassificationVoteAlreadyExistsForJudge)
	require.ErrorIs(t,
		handler.HandleAnswerClassificationVote(roundKey, nil),
		ErrNilAnswerClassificationVote,
	)

	wrongRoundVote := plumbingClassificationVote(roundKey, "judge-b")
	wrongRoundVote.Round++
	require.ErrorIs(t,
		handler.HandleAnswerClassificationVote(roundKey, wrongRoundVote),
		ErrAnswerClassificationVoteMismatch,
	)
}

func TestMiniRoundTwoHandler_HandleAnswerClassificationCertificate(t *testing.T) {
	t.Parallel()

	roundKey := createTestRoundKey()
	roundState := state.NewRoundState()
	handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{roundState: roundState})
	certificate := plumbingClassificationCertificate(roundKey)

	err := handler.HandleAnswerClassificationCertificate(roundKey, certificate)
	require.NoError(t, err)
	stored, err := roundState.GetAnswerClassificationCertificate(roundKey)
	require.NoError(t, err)
	require.Same(t, certificate, stored)

	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, nil),
		ErrNilAnswerClassificationCertificate,
	)

	misaligned := plumbingClassificationCertificate(roundKey)
	misaligned.Votes[0].AnswerEvidenceHash = []byte("different-evidence")
	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, misaligned),
		ErrAnswerClassificationCertificateMismatch,
	)

	missingResults := plumbingClassificationCertificate(roundKey)
	missingResults.Transactions = nil
	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, missingResults),
		ErrAnswerClassificationCertificateMismatch,
	)
}

func plumbingClassificationVote(roundKey data.RoundKey, judgeID string) *data.AnswerClassificationVote {
	return &data.AnswerClassificationVote{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		JudgeID:            judgeID,
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		Assignments: []data.AnswerClassificationAssignment{
			{Category: data.AnswerCategoryCorrect},
		},
	}
}

func plumbingClassificationCertificate(roundKey data.RoundKey) *data.AnswerClassificationCertificate {
	return &data.AnswerClassificationCertificate{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		SenderID:           "leader",
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		Votes: []data.AnswerClassificationVote{
			*plumbingClassificationVote(roundKey, "judge-a"),
		},
		Transactions: []data.TransactionAnswerClassification{
			{
				TxHash: []byte("tx-a"),
				Status: data.TransactionAnswerStatusInsufficientCorrectAnswers,
			},
		},
	}
}
