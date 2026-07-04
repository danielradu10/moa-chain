package consensus

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/testscommon"
)

func TestRoundHandler_RoutesAnswerClassificationVote(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	vote := routingClassificationVote(roundKey)
	miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{}
	handler := createTestRoundHandler(
		"leader", data.StepCollectClassificationVotes, roundKey, miniRoundTwoHandler,
	)

	err := handler.HandleMessage(data.ConsensusMessage{
		ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
		AnswerClassificationVote: vote,
	})

	require.NoError(t, err)
	require.True(t, miniRoundTwoHandler.HandleAnswerClassificationVoteCalled)
	require.Equal(t, roundKey, miniRoundTwoHandler.HandleAnswerClassificationVoteKey)
	require.Same(t, vote, miniRoundTwoHandler.HandleAnswerClassificationVoteValue)
}

func TestRoundHandler_RoutesAnswerClassificationCertificate(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	certificate := routingClassificationCertificate(roundKey)
	miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{}
	handler := createTestRoundHandler(
		"validator", data.StepAwaitClassificationCertificate, roundKey, miniRoundTwoHandler,
	)

	err := handler.HandleMessage(data.ConsensusMessage{
		ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
		AnswerClassificationCertificate: certificate,
	})

	require.NoError(t, err)
	require.True(t, miniRoundTwoHandler.HandleAnswerClassificationCertificateCalled)
	require.Equal(t, roundKey, miniRoundTwoHandler.HandleAnswerClassificationCertificateKey)
	require.Same(t, certificate, miniRoundTwoHandler.HandleAnswerClassificationCertificateValue)
}

func TestRoundHandler_RoutesAnswerEvidenceToInactiveClassificationPath(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	tests := []struct {
		name         string
		selfID       string
		leaderID     string
		handlerError error
		expectedStep data.Step
	}{
		{
			name:         "non-leader waits for classification certificate",
			selfID:       "validator",
			leaderID:     "leader",
			expectedStep: data.StepAwaitClassificationCertificate,
		},
		{
			name:         "leader collects classification votes",
			selfID:       "leader",
			leaderID:     "leader",
			expectedStep: data.StepCollectClassificationVotes,
		},
		{
			name:         "judging failure fails step",
			selfID:       "validator",
			leaderID:     "leader",
			handlerError: ErrAnswerJudgingTimeout,
			expectedStep: data.StepFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{
				HandleAnswerEvidenceForClassificationErr: test.handlerError,
			}
			handler := createTestRoundHandler(
				test.selfID, data.StepAwaitAnswerEvidence, roundKey, miniRoundTwoHandler,
			)
			evidence := &data.AggregatedExecutionResultsMessage{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
				SenderID: test.leaderID,
			}

			err := handler.HandleMessage(data.ConsensusMessage{
				ConsensusMessageType:       data.AggregatedExecutionResultsConsensusMessage,
				AggregatedExecutionResults: evidence,
			})

			if test.handlerError == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.handlerError)
			}
			require.True(t, miniRoundTwoHandler.HandleAnswerEvidenceForClassificationCalled)
			require.Same(t, evidence, miniRoundTwoHandler.HandleAnswerEvidenceForClassificationMessage)
			require.Equal(t, test.expectedStep, handler.currentStep)
		})
	}
}

func TestRoundHandler_RejectsInvalidClassificationRouting(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	tests := []struct {
		name        string
		step        data.Step
		message     data.ConsensusMessage
		targetError error
	}{
		{
			name: "nil vote",
			step: data.StepCollectClassificationVotes,
			message: data.ConsensusMessage{
				ConsensusMessageType: data.AnswerClassificationVoteConsensusMessage,
			},
			targetError: ErrNilAnswerClassificationVote,
		},
		{
			name: "nil certificate",
			step: data.StepAwaitClassificationCertificate,
			message: data.ConsensusMessage{
				ConsensusMessageType: data.AnswerClassificationCertificateConsensusMessage,
			},
			targetError: ErrNilAnswerClassificationCertificate,
		},
		{
			name: "vote during wrong step",
			step: data.StepAwaitClassificationCertificate,
			message: data.ConsensusMessage{
				ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
				AnswerClassificationVote: routingClassificationVote(roundKey),
			},
			targetError: ErrUnexpectedMessageForStep,
		},
		{
			name: "vote for different round",
			step: data.StepCollectClassificationVotes,
			message: data.ConsensusMessage{
				ConsensusMessageType: data.AnswerClassificationVoteConsensusMessage,
				AnswerClassificationVote: routingClassificationVote(data.RoundKey{
					Epoch: roundKey.Epoch, Round: roundKey.Round + 1, MiniRound: roundKey.MiniRound,
				}),
			},
			targetError: ErrMessageForDifferentRound,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			handler := createTestRoundHandler(
				"validator", test.step, roundKey, &testscommon.MiniRoundTwoHandlerStub{},
			)

			err := handler.HandleMessage(test.message)

			require.ErrorIs(t, err, test.targetError)
		})
	}
}

func TestRoundHandler_IgnoresClassificationVoteForFinalizedRound(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{}
	handler := createTestRoundHandler("validator", data.StepFinished, roundKey, miniRoundTwoHandler)
	err := handler.blockFinalizer.FinalizeBlockMRTwo(roundKey, &data.BlockOnChain{})
	require.NoError(t, err)

	err = handler.HandleMessage(data.ConsensusMessage{
		ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
		AnswerClassificationVote: routingClassificationVote(roundKey),
	})

	require.NoError(t, err)
	require.False(t, miniRoundTwoHandler.HandleAnswerClassificationVoteCalled)
}

func TestRoundHandler_ClassificationStepTimeouts(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundTwoRoundKey()
	tests := []struct {
		step        data.Step
		targetError error
	}{
		{step: data.StepAwaitAnswerEvidence, targetError: ErrAnswerEvidenceTimeout},
		{step: data.StepJudgeAnswers, targetError: ErrAnswerJudgingTimeout},
		{step: data.StepCollectClassificationVotes, targetError: ErrNotEnoughAnswerClassificationVotes},
		{step: data.StepAwaitClassificationCertificate, targetError: ErrAnswerClassificationCertificateTimeout},
	}

	for _, test := range tests {
		handler := createTestRoundHandler(
			"validator", test.step, roundKey, &testscommon.MiniRoundTwoHandlerStub{},
		)

		err := handler.OnTimeout(roundKey, test.step)

		require.ErrorIs(t, err, test.targetError)
		require.Equal(t, data.StepFailed, handler.currentStep)
	}
}

func routingClassificationVote(roundKey data.RoundKey) *data.AnswerClassificationVote {
	return &data.AnswerClassificationVote{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		JudgeID: "judge-a", AnswerEvidenceHash: []byte("evidence"), PromptVersion: "judge-v1",
	}
}

func routingClassificationCertificate(roundKey data.RoundKey) *data.AnswerClassificationCertificate {
	return &data.AnswerClassificationCertificate{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		SenderID: "leader", AnswerEvidenceHash: []byte("evidence"), PromptVersion: "judge-v1",
	}
}
