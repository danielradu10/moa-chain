package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestRoundState_AnswerClassificationVotes(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	roundState := NewRoundState()
	voteB := &data.AnswerClassificationVote{JudgeID: "judge-b"}
	voteA := &data.AnswerClassificationVote{JudgeID: "judge-a"}

	require.NoError(t, roundState.AddAnswerClassificationVote(roundKey, voteB))
	require.NoError(t, roundState.AddAnswerClassificationVote(roundKey, voteA))

	votes, err := roundState.GetAnswerClassificationVotes(roundKey)
	require.NoError(t, err)
	require.Equal(t, []*data.AnswerClassificationVote{voteA, voteB}, votes)
	require.ErrorIs(t,
		roundState.AddAnswerClassificationVote(roundKey, voteA),
		ErrAnswerClassificationVoteAlreadyExistsForJudge,
	)
	require.ErrorIs(t,
		roundState.AddAnswerClassificationVote(roundKey, nil),
		ErrNilAnswerClassificationVote,
	)
}

func TestRoundState_AnswerEvidence(t *testing.T) {
	t.Parallel()

	roundState := NewRoundState()
	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: 1}
	evidence := &data.AggregatedExecutionResultsMessage{SenderID: "leader"}

	require.NoError(t, roundState.SetAnswerEvidence(roundKey, evidence))
	stored, err := roundState.GetAnswerEvidence(roundKey)
	require.NoError(t, err)
	require.Same(t, evidence, stored)
	require.ErrorIs(t, roundState.SetAnswerEvidence(roundKey, evidence), ErrAnswerEvidenceAlreadyExists)

	roundState.ClearRoundState(roundKey)
	stored, err = roundState.GetAnswerEvidence(roundKey)
	require.Nil(t, stored)
	require.ErrorIs(t, err, ErrNoAnswerEvidenceForCurrentRoundKey)
}

func TestRoundState_AnswerClassificationCertificate(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	roundState := NewRoundState()
	certificate := &data.AnswerClassificationCertificate{SenderID: "leader"}

	require.NoError(t, roundState.SetAnswerClassificationCertificate(roundKey, certificate))
	stored, err := roundState.GetAnswerClassificationCertificate(roundKey)
	require.NoError(t, err)
	require.Same(t, certificate, stored)
	require.ErrorIs(t,
		roundState.SetAnswerClassificationCertificate(roundKey, certificate),
		ErrAnswerClassificationCertificateAlreadyExists,
	)
	require.ErrorIs(t,
		roundState.SetAnswerClassificationCertificate(data.RoundKey{}, nil),
		ErrNilAnswerClassificationCertificate,
	)
}

func TestRoundState_ClearAnswerClassificationState(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	roundState := NewRoundState()
	require.NoError(t, roundState.AddAnswerClassificationVote(
		roundKey,
		&data.AnswerClassificationVote{JudgeID: "judge-a"},
	))
	require.NoError(t, roundState.SetAnswerClassificationCertificate(
		roundKey,
		&data.AnswerClassificationCertificate{SenderID: "leader"},
	))

	roundState.ClearRoundState(roundKey)

	votes, err := roundState.GetAnswerClassificationVotes(roundKey)
	require.Nil(t, votes)
	require.ErrorIs(t, err, ErrNoAnswerClassificationVotesForCurrentRoundKey)
	certificate, err := roundState.GetAnswerClassificationCertificate(roundKey)
	require.Nil(t, certificate)
	require.ErrorIs(t, err, ErrNoAnswerClassificationCertificateForCurrentRoundKey)
}
