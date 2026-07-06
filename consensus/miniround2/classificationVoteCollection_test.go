package miniround2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/hashing"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
)

func TestHandleAnswerClassificationVoteBroadcastsCertificateAtQuorum(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))

	leaderVote := classificationLeaderVote(t, context)
	validatorBVote := signedClassificationVote(t, leaderVote, "validator-b", context.memberSigners["validator-b"])
	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, validatorBVote))
	require.Nil(t, context.broadcaster.BroadcastAnswerClassificationCertificateMessage)

	validatorAVote := signedClassificationVote(t, leaderVote, "validator-a", context.memberSigners["validator-a"])
	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, validatorAVote))

	message := context.broadcaster.BroadcastAnswerClassificationCertificateMessage
	require.NotNil(t, message)
	require.Equal(t, data.AnswerClassificationCertificateConsensusMessage, message.ConsensusMessageType)
	require.Equal(t, "leader", context.broadcaster.BroadcastAnswerClassificationCertificateMyID)
	require.Equal(t, []string{"leader", "validator-a", "validator-b"}, context.broadcaster.BroadcastAnswerClassificationCertificateTargets)

	certificate := message.AnswerClassificationCertificate
	require.NotNil(t, certificate)
	require.Equal(t, []string{"leader", "validator-a", "validator-b"}, classificationJudgeIDs(certificate.Votes))
	expected, err := AggregateClassificationVotes(
		assignmentCandidateIDs(leaderVote.Assignments), certificate.Votes, 3,
	)
	require.NoError(t, err)
	require.Equal(t, expected, certificate.Transactions)
	require.True(t, context.roundState.IsAnswerClassificationCertificateSet(context.roundKey))
}

func TestHandleAnswerClassificationVoteRejectsInvalidVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*data.AnswerClassificationVote)
		resign      bool
		targetError error
	}{
		{
			name:        "judge outside committee",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.JudgeID = "outsider" },
			targetError: ErrValidatorNotPartOfConsensusGroup,
		},
		{
			name:        "invalid signature",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.Signature[0] ^= 0xff },
			targetError: signing.ErrWrongSignature,
		},
		{
			name:        "different evidence",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.AnswerEvidenceHash = []byte("different-evidence") },
			resign:      true,
			targetError: ErrClassificationVoteContextMismatch,
		},
		{
			name:        "different prompt version",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.PromptVersion = "different-version" },
			resign:      true,
			targetError: ErrClassificationVotePromptMismatch,
		},
		{
			name:        "different prompt hash",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.PromptHash = []byte("different-prompt") },
			resign:      true,
			targetError: ErrClassificationVotePromptMismatch,
		},
		{
			name: "missing candidate",
			mutate: func(vote *data.AnswerClassificationVote) {
				vote.Assignments = vote.Assignments[:len(vote.Assignments)-1]
			},
			resign:      true,
			targetError: ErrMissingAnswerCandidate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
			require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
			vote := signedClassificationVote(t, classificationLeaderVote(t, context), "validator-a", context.memberSigners["validator-a"])
			test.mutate(vote)
			if test.resign {
				signClassificationVote(t, vote, context.memberSigners["validator-a"])
			}

			err := context.handler.HandleAnswerClassificationVote(context.roundKey, vote)

			require.ErrorIs(t, err, test.targetError)
			votes, getErr := context.roundState.GetAnswerClassificationVotes(context.roundKey)
			require.NoError(t, getErr)
			require.Len(t, votes, 1)
		})
	}
}

func TestHandleAnswerClassificationVoteRejectsDuplicateJudge(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
	vote := signedClassificationVote(t, classificationLeaderVote(t, context), "validator-a", context.memberSigners["validator-a"])

	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	require.ErrorIs(
		t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote),
		state.ErrAnswerClassificationVoteAlreadyExistsForJudge,
	)
}

func TestClassificationCertificateIsIndependentOfVoteArrivalOrder(t *testing.T) {
	t.Parallel()

	first := classificationCertificateForArrivalOrder(t, []string{"validator-a", "validator-b"})
	second := classificationCertificateForArrivalOrder(t, []string{"validator-b", "validator-a"})

	require.Equal(t, first.Transactions, second.Transactions)
	require.Equal(t, classificationJudgeIDs(first.Votes), classificationJudgeIDs(second.Votes))
}

func classificationCertificateForArrivalOrder(
	t *testing.T,
	judgeIDs []string,
) *data.AnswerClassificationCertificate {
	t.Helper()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
	leaderVote := classificationLeaderVote(t, context)
	for _, judgeID := range judgeIDs {
		vote := signedClassificationVote(t, leaderVote, judgeID, context.memberSigners[judgeID])
		require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	}

	return context.broadcaster.BroadcastAnswerClassificationCertificateMessage.AnswerClassificationCertificate
}

func classificationLeaderVote(
	t *testing.T,
	context classificationProductionContext,
) *data.AnswerClassificationVote {
	t.Helper()

	votes, err := context.roundState.GetAnswerClassificationVotes(context.roundKey)
	require.NoError(t, err)
	require.Len(t, votes, 1)

	return votes[0]
}

func signedClassificationVote(
	t *testing.T,
	template *data.AnswerClassificationVote,
	judgeID string,
	signer signing.MessageSigner,
) *data.AnswerClassificationVote {
	t.Helper()

	vote := *template
	vote.JudgeID = judgeID
	vote.ModelMetadata = "test-model-" + judgeID
	vote.Assignments = append([]data.AnswerClassificationAssignment(nil), template.Assignments...)
	signClassificationVote(t, &vote, signer)

	return &vote
}

func signClassificationVote(
	t *testing.T,
	vote *data.AnswerClassificationVote,
	signer signing.MessageSigner,
) {
	t.Helper()

	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	require.NoError(t, err)
	signature, err := signer.Sign(voteHash)
	require.NoError(t, err)
	vote.VoteHash = voteHash
	vote.Signature = signature
}

func classificationJudgeIDs(votes []data.AnswerClassificationVote) []string {
	judgeIDs := make([]string, len(votes))
	for index := range votes {
		judgeIDs[index] = votes[index].JudgeID
	}

	return judgeIDs
}
