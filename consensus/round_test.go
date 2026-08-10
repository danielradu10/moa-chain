package consensus

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/data"
	"moa-chain/testscommon"
)

func TestRoundHandler_StartRoundMiniRoundTwo(t *testing.T) {
	t.Parallel()

	t.Run("should execute block and collect execution results when local node is leader", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{
			HandleConsensusSelectionLeader: "validator-1",
		}
		handler := createTestRoundHandler("validator-1", data.StepIdle, roundKey, miniRoundTwoHandler)

		err := handler.StartRound(roundKey)

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleConsensusSelectionCalled)
		require.Equal(t, roundKey, miniRoundTwoHandler.HandleConsensusSelectionKey)
		require.True(t, miniRoundTwoHandler.HandleBlockExecutionCalled)
		require.Equal(t, roundKey, miniRoundTwoHandler.HandleBlockExecutionKey)
		require.Equal(t, data.StepCollectExecutionResults, handler.currentStep)
	})

	t.Run("should execute block and await answer evidence when local node is not leader", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{
			HandleConsensusSelectionLeader: "validator-2",
		}
		handler := createTestRoundHandler("validator-1", data.StepIdle, roundKey, miniRoundTwoHandler)

		err := handler.StartRound(roundKey)

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleConsensusSelectionCalled)
		require.True(t, miniRoundTwoHandler.HandleBlockExecutionCalled)
		require.Equal(t, data.StepAwaitAnswerEvidence, handler.currentStep)
	})
}

func TestRoundHandler_HandleMiniRoundTwoMessages(t *testing.T) {
	t.Parallel()

	t.Run("should route executed prompts to mini-round two handler", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		executedPrompts := &data.AnswersBlockMessage{
			Epoch:     roundKey.Epoch,
			Round:     roundKey.Round,
			MiniRound: roundKey.MiniRound,
			SenderID:  "validator-2",
		}
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{}
		handler := createTestRoundHandler("validator-1", data.StepCollectExecutionResults, roundKey, miniRoundTwoHandler)

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ExecutedPromptsMessage,
			ExecutedPrompts:      executedPrompts,
		})

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleExecutedPromptsMessageCalled)
		require.Equal(t, roundKey, miniRoundTwoHandler.HandleExecutedPromptsMessageKey)
		require.Same(t, executedPrompts, miniRoundTwoHandler.HandleExecutedPromptsMessageMessage)
	})

	t.Run("should reject executed prompts when not collecting execution results", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		handler := createTestRoundHandler("validator-1", data.StepAwaitAnswerEvidence, roundKey, &testscommon.MiniRoundTwoHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ExecutedPromptsMessage,
			ExecutedPrompts: &data.AnswersBlockMessage{
				Epoch:     roundKey.Epoch,
				Round:     roundKey.Round,
				MiniRound: roundKey.MiniRound,
			},
		})

		require.Equal(t, ErrUnexpectedMessageForStep, err)
	})
}

func TestRoundHandler_MiniRoundOneToMiniRoundTwoTransition(t *testing.T) {
	t.Parallel()

	t.Run("should start mini-round two after aggregated votes finalize mini-round one", func(t *testing.T) {
		t.Parallel()

		miniRoundOneRoundKey := createMiniRoundOneRoundKey()
		miniRoundTwoRoundKey := createMiniRoundTwoRoundKey()
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{
			HandleConsensusSelectionLeader: "validator-2",
		}
		handler := createTestRoundHandler("validator-1", data.StepAwaitAggregatedVotes, miniRoundOneRoundKey, miniRoundTwoHandler)
		err := handler.blockFinalizer.FinalizeBlockMROne(miniRoundOneRoundKey, createTestFinalizedMiniRoundOneBlock())
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.AggregatedVotesConsensusMessage,
			AggregatedVotes: &data.AggregatedVotes{
				Epoch:     miniRoundOneRoundKey.Epoch,
				Round:     miniRoundOneRoundKey.Round,
				MiniRound: miniRoundOneRoundKey.MiniRound,
				SenderID:  "validator-2",
			},
		})

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleConsensusSelectionCalled)
		require.Equal(t, miniRoundTwoRoundKey, miniRoundTwoHandler.HandleConsensusSelectionKey)
		require.True(t, miniRoundTwoHandler.HandleBlockExecutionCalled)
		require.Equal(t, miniRoundTwoRoundKey, miniRoundTwoHandler.HandleBlockExecutionKey)
		require.Equal(t, miniRoundTwoRoundKey, handler.currentRoundKey)
		require.Equal(t, data.StepAwaitAnswerEvidence, handler.currentStep)
	})

	t.Run("should start mini-round two for leader only after mini-round one block vote finalizes", func(t *testing.T) {
		t.Parallel()

		miniRoundOneRoundKey := createMiniRoundOneRoundKey()
		miniRoundTwoRoundKey := createMiniRoundTwoRoundKey()
		finalizer := blockFinalizer.NewFinalizeBlockComponent()
		miniRoundOneHandler := &testscommon.MiniRoundOneHandlerStub{
			HandleBlockVoteErr: nil,
		}
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{
			HandleConsensusSelectionLeader: "validator-1",
		}
		handler := NewRoundHandler(RoundHandlerArgs{
			SelfID:              "validator-1",
			CurrentStep:         data.StepCollectVotes,
			CurrentRoundKey:     miniRoundOneRoundKey,
			MiniRoundOneHandler: miniRoundOneHandler,
			MiniRoundTwoHandler: miniRoundTwoHandler,
			BlockFinalizer:      finalizer,
		})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.BlockVoteConsensusMessage,
			BlockVote: &data.BlockVote{
				Epoch:     miniRoundOneRoundKey.Epoch,
				Round:     miniRoundOneRoundKey.Round,
				MiniRound: miniRoundOneRoundKey.MiniRound,
				SignerID:  "validator-2",
			},
		})

		require.NoError(t, err)
		require.False(t, miniRoundTwoHandler.HandleConsensusSelectionCalled)
		require.Equal(t, data.StepCollectVotes, handler.currentStep)

		err = finalizer.FinalizeBlockMROne(miniRoundOneRoundKey, createTestFinalizedMiniRoundOneBlock())
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.BlockVoteConsensusMessage,
			BlockVote: &data.BlockVote{
				Epoch:     miniRoundOneRoundKey.Epoch,
				Round:     miniRoundOneRoundKey.Round,
				MiniRound: miniRoundOneRoundKey.MiniRound,
				SignerID:  "validator-3",
			},
		})

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleConsensusSelectionCalled)
		require.Equal(t, miniRoundTwoRoundKey, miniRoundTwoHandler.HandleConsensusSelectionKey)
		require.Equal(t, data.StepCollectExecutionResults, handler.currentStep)
	})
}

func createTestRoundHandler(
	selfID string,
	currentStep data.Step,
	currentRoundKey data.RoundKey,
	miniRoundTwoHandler *testscommon.MiniRoundTwoHandlerStub,
) *roundHandler {
	return NewRoundHandler(RoundHandlerArgs{
		SelfID:              selfID,
		CurrentStep:         currentStep,
		CurrentRoundKey:     currentRoundKey,
		MiniRoundOneHandler: &testscommon.MiniRoundOneHandlerStub{},
		MiniRoundTwoHandler: miniRoundTwoHandler,
		BlockFinalizer:      blockFinalizer.NewFinalizeBlockComponent(),
	})
}

func createMiniRoundOneRoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
}

func createMiniRoundTwoRoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
}

func createTestFinalizedMiniRoundOneBlock() *data.BlockOnChain {
	return &data.BlockOnChain{
		SubdomainsFrequencies: data.SubdomainsFrequency{
			"ml_ai_engineering": 1,
		},
	}
}

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
	require.Equal(t, data.StepFinished, handler.currentStep)
}

func TestRoundHandler_RoutesAnswerEvidenceToClassification(t *testing.T) {
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
				HandleAnswerEvidenceErr: test.handlerError,
			}
			handler := createTestRoundHandler(
				test.selfID, data.StepAwaitAnswerEvidence, roundKey, miniRoundTwoHandler,
			)
			evidence := &data.AggregatedExecutionResultsMessage{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
				SenderID: test.leaderID,
			}

			err := handler.HandleMessage(data.ConsensusMessage{
				ConsensusMessageType: data.AnswerEvidenceConsensusMessage,
				AnswerEvidence:       evidence,
			})

			if test.handlerError == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, test.handlerError)
			}
			require.True(t, miniRoundTwoHandler.HandleAnswerEvidenceCalled)
			require.Same(t, evidence, miniRoundTwoHandler.HandleAnswerEvidenceMessage)
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

func TestRoundHandler_StartRoundMiniRoundThree(t *testing.T) {
	t.Parallel()

	t.Run("leader synthesises and awaits votes", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionLeader: "validator-1",
		}
		handler := createTestRoundHandlerMR3("validator-1", data.StepIdle, roundKey, mr3)

		err := handler.StartRound(roundKey)

		require.NoError(t, err)
		require.True(t, mr3.HandleConsensusSelectionCalled)
		require.Equal(t, roundKey, mr3.HandleConsensusSelectionKey)
		require.True(t, mr3.HandleSynthesisCalled)
		require.Equal(t, roundKey, mr3.HandleSynthesisKey)
		require.Equal(t, data.StepCollectSynthesisVotes, handler.currentStep)
	})

	t.Run("non-leader awaits proposed synthesis", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionLeader: "validator-2",
		}
		handler := createTestRoundHandlerMR3("validator-1", data.StepIdle, roundKey, mr3)

		err := handler.StartRound(roundKey)

		require.NoError(t, err)
		require.True(t, mr3.HandleConsensusSelectionCalled)
		require.False(t, mr3.HandleSynthesisCalled)
		require.Equal(t, data.StepAwaitProposedSynthesis, handler.currentStep)
	})

	t.Run("consensus selection error propagates", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		expectedErr := ErrUnexpectedMessageForStep
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionErr: expectedErr,
		}
		handler := createTestRoundHandlerMR3("validator-1", data.StepIdle, roundKey, mr3)

		err := handler.StartRound(roundKey)

		require.ErrorIs(t, err, expectedErr)
		require.False(t, mr3.HandleSynthesisCalled)
	})

	t.Run("synthesis error sets step to failed", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		expectedErr := ErrNilProposedSynthesisMessage
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionLeader: "validator-1",
			HandleSynthesisErr:             expectedErr,
		}
		handler := createTestRoundHandlerMR3("validator-1", data.StepIdle, roundKey, mr3)

		err := handler.StartRound(roundKey)

		require.ErrorIs(t, err, expectedErr)
		require.Equal(t, data.StepFailed, handler.currentStep)
	})
}

func TestRoundHandler_MiniRoundTwoToMiniRoundThreeTransition(t *testing.T) {
	t.Parallel()

	t.Run("no MR3 handler wired stops at StepFinished", func(t *testing.T) {
		t.Parallel()

		mr2RoundKey := createMiniRoundTwoRoundKey()
		mr2Handler := &testscommon.MiniRoundTwoHandlerStub{}
		handler := createTestRoundHandler("validator", data.StepAwaitClassificationCertificate, mr2RoundKey, mr2Handler)
		err := handler.blockFinalizer.FinalizeBlockMRTwo(mr2RoundKey, &data.BlockOnChain{})
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
			AnswerClassificationCertificate: routingClassificationCertificate(mr2RoundKey),
		})

		require.NoError(t, err)
		require.Equal(t, data.StepFinished, handler.currentStep)
	})

	t.Run("all INSUFFICIENT_CORRECT_ANSWERS skips MR3", func(t *testing.T) {
		t.Parallel()

		mr2RoundKey := createMiniRoundTwoRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionLeader: "validator-2",
		}
		handler := createFullRoundHandler("validator-1", data.StepAwaitClassificationCertificate, mr2RoundKey, mr3)
		mr2Block := &data.BlockOnChain{
			AnswerClassifications: []data.TransactionAnswerClassification{
				{TxHash: []byte("tx1"), Status: data.TransactionAnswerStatusInsufficientCorrectAnswers},
				{TxHash: []byte("tx2"), Status: data.TransactionAnswerStatusInsufficientCorrectAnswers},
			},
		}
		err := handler.blockFinalizer.FinalizeBlockMRTwo(mr2RoundKey, mr2Block)
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
			AnswerClassificationCertificate: routingClassificationCertificate(mr2RoundKey),
		})

		require.NoError(t, err)
		require.Equal(t, data.StepFinished, handler.currentStep)
		require.False(t, mr3.HandleConsensusSelectionCalled)
	})

	t.Run("eligible MR2 transactions trigger MR3 start", func(t *testing.T) {
		t.Parallel()

		mr2RoundKey := createMiniRoundTwoRoundKey()
		mr3RoundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{
			HandleConsensusSelectionLeader: "validator-2",
		}
		handler := createFullRoundHandler("validator-1", data.StepAwaitClassificationCertificate, mr2RoundKey, mr3)
		mr2Block := &data.BlockOnChain{
			AnswerClassifications: []data.TransactionAnswerClassification{
				{TxHash: []byte("tx1"), Status: data.TransactionAnswerStatusReadyForMiniRoundThree},
			},
		}
		err := handler.blockFinalizer.FinalizeBlockMRTwo(mr2RoundKey, mr2Block)
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
			AnswerClassificationCertificate: routingClassificationCertificate(mr2RoundKey),
		})

		require.NoError(t, err)
		require.True(t, mr3.HandleConsensusSelectionCalled)
		require.Equal(t, mr3RoundKey, mr3.HandleConsensusSelectionKey)
		require.Equal(t, mr3RoundKey, handler.currentRoundKey)
		require.Equal(t, data.StepAwaitProposedSynthesis, handler.currentStep)
	})
}

func TestRoundHandler_HandleProposedSynthesis(t *testing.T) {
	t.Parallel()

	t.Run("happy path routes message and advances to await aggregated votes", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{}
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitProposedSynthesis, roundKey, mr3)
		msg := &data.ProposedSynthesisMessage{
			Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			SenderID: "leader",
		}

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
			ProposedSynthesis:    msg,
		})

		require.NoError(t, err)
		require.True(t, mr3.HandleProposedSynthesisCalled)
		require.Equal(t, roundKey, mr3.HandleProposedSynthesisKey)
		require.Same(t, msg, mr3.HandleProposedSynthesisMsg)
		require.Equal(t, data.StepAwaitAggregatedSynthesisVotes, handler.currentStep)
	})

	t.Run("nil message returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitProposedSynthesis, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
		})

		require.ErrorIs(t, err, ErrNilProposedSynthesisMessage)
	})

	t.Run("wrong step returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepCollectSynthesisVotes, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
			ProposedSynthesis: &data.ProposedSynthesisMessage{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrUnexpectedMessageForStep)
	})

	t.Run("different round returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitProposedSynthesis, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
			ProposedSynthesis: &data.ProposedSynthesisMessage{
				Epoch: roundKey.Epoch, Round: roundKey.Round + 1, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrMessageForDifferentRound)
	})

	t.Run("handler error sets step to failed", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		expectedErr := ErrNilSynthesisVoteMessage
		mr3 := &testscommon.MiniRoundThreeHandlerStub{HandleProposedSynthesisErr: expectedErr}
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitProposedSynthesis, roundKey, mr3)

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
			ProposedSynthesis: &data.ProposedSynthesisMessage{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, expectedErr)
		require.Equal(t, data.StepFailed, handler.currentStep)
	})
}

func TestRoundHandler_HandleSynthesisVote(t *testing.T) {
	t.Parallel()

	t.Run("vote below quorum keeps collecting", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{}
		handler := createTestRoundHandlerMR3("leader", data.StepCollectSynthesisVotes, roundKey, mr3)

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
			SynthesisVote: &data.SynthesisVote{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
				VoterID: "validator-1",
			},
		})

		require.NoError(t, err)
		require.True(t, mr3.HandleSynthesisVoteCalled)
		require.Equal(t, data.StepCollectSynthesisVotes, handler.currentStep)
	})

	t.Run("vote at quorum (MR3 finalized) sets step to finished", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{}
		handler := createTestRoundHandlerMR3("leader", data.StepCollectSynthesisVotes, roundKey, mr3)
		err := handler.blockFinalizer.FinalizeBlockMRThree(roundKey, &data.BlockOnChain{})
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
			SynthesisVote: &data.SynthesisVote{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
				VoterID: "validator-1",
			},
		})

		require.NoError(t, err)
		require.True(t, mr3.HandleSynthesisVoteCalled)
		require.Equal(t, data.StepFinished, handler.currentStep)
	})

	t.Run("nil vote returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("leader", data.StepCollectSynthesisVotes, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
		})

		require.ErrorIs(t, err, ErrNilSynthesisVoteMessage)
	})

	t.Run("wrong step returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("leader", data.StepAwaitProposedSynthesis, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
			SynthesisVote: &data.SynthesisVote{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrUnexpectedMessageForStep)
	})

	t.Run("different round returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("leader", data.StepCollectSynthesisVotes, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
			SynthesisVote: &data.SynthesisVote{
				Epoch: roundKey.Epoch, Round: roundKey.Round + 1, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrMessageForDifferentRound)
	})

	t.Run("vote for already-finalized MR3 is silently ignored", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{}
		handler := createTestRoundHandlerMR3("leader", data.StepFinished, roundKey, mr3)
		err := handler.blockFinalizer.FinalizeBlockMRThree(roundKey, &data.BlockOnChain{})
		require.NoError(t, err)

		err = handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.SynthesisVoteConsensusMessage,
			SynthesisVote: &data.SynthesisVote{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
				VoterID: "validator-1",
			},
		})

		require.NoError(t, err)
		require.False(t, mr3.HandleSynthesisVoteCalled)
	})
}

func TestRoundHandler_HandleAggregatedSynthesisVotes(t *testing.T) {
	t.Parallel()

	t.Run("happy path finalizes MR3", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		mr3 := &testscommon.MiniRoundThreeHandlerStub{}
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitAggregatedSynthesisVotes, roundKey, mr3)
		msg := &data.AggregatedSynthesisVotes{
			Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			SenderID: "leader",
		}

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType:     data.AggregatedSynthesisVotesConsensusMessage,
			AggregatedSynthesisVotes: msg,
		})

		require.NoError(t, err)
		require.True(t, mr3.HandleAggregatedSynthesisVotesCalled)
		require.Equal(t, roundKey, mr3.HandleAggregatedSynthesisVotesKey)
		require.Same(t, msg, mr3.HandleAggregatedSynthesisVotesMsg)
		require.Equal(t, data.StepFinished, handler.currentStep)
	})

	t.Run("nil message returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitAggregatedSynthesisVotes, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.AggregatedSynthesisVotesConsensusMessage,
		})

		require.ErrorIs(t, err, ErrNilAggregatedSynthesisVotesMessage)
	})

	t.Run("wrong step returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitProposedSynthesis, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.AggregatedSynthesisVotesConsensusMessage,
			AggregatedSynthesisVotes: &data.AggregatedSynthesisVotes{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrUnexpectedMessageForStep)
	})

	t.Run("different round returns error", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitAggregatedSynthesisVotes, roundKey, &testscommon.MiniRoundThreeHandlerStub{})

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.AggregatedSynthesisVotesConsensusMessage,
			AggregatedSynthesisVotes: &data.AggregatedSynthesisVotes{
				Epoch: roundKey.Epoch, Round: roundKey.Round + 1, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, ErrMessageForDifferentRound)
	})

	t.Run("handler error sets step to failed", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundThreeRoundKey()
		expectedErr := ErrNilProposedSynthesisMessage
		mr3 := &testscommon.MiniRoundThreeHandlerStub{HandleAggregatedSynthesisVotesErr: expectedErr}
		handler := createTestRoundHandlerMR3("validator-1", data.StepAwaitAggregatedSynthesisVotes, roundKey, mr3)

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType: data.AggregatedSynthesisVotesConsensusMessage,
			AggregatedSynthesisVotes: &data.AggregatedSynthesisVotes{
				Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
			},
		})

		require.ErrorIs(t, err, expectedErr)
		require.Equal(t, data.StepFailed, handler.currentStep)
	})
}

func TestRoundHandler_MiniRoundThreeTimeouts(t *testing.T) {
	t.Parallel()

	roundKey := createMiniRoundThreeRoundKey()
	tests := []struct {
		step        data.Step
		targetError error
	}{
		{step: data.StepAwaitProposedSynthesis, targetError: ErrProposedSynthesisTimeout},
		{step: data.StepCollectSynthesisVotes, targetError: ErrSynthesisVotesTimeout},
		{step: data.StepAwaitAggregatedSynthesisVotes, targetError: ErrAggregatedSynthesisVotesTimeout},
	}

	for _, test := range tests {
		handler := createTestRoundHandlerMR3(
			"validator", test.step, roundKey, &testscommon.MiniRoundThreeHandlerStub{},
		)

		err := handler.OnTimeout(roundKey, test.step)

		require.ErrorIs(t, err, test.targetError)
		require.Equal(t, data.StepFailed, handler.currentStep)
	}
}

func createMiniRoundThreeRoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}
}

func createTestRoundHandlerMR3(
	selfID string,
	currentStep data.Step,
	currentRoundKey data.RoundKey,
	mr3Handler *testscommon.MiniRoundThreeHandlerStub,
) *roundHandler {
	return NewRoundHandler(RoundHandlerArgs{
		SelfID:                selfID,
		CurrentStep:           currentStep,
		CurrentRoundKey:       currentRoundKey,
		MiniRoundOneHandler:   &testscommon.MiniRoundOneHandlerStub{},
		MiniRoundTwoHandler:   &testscommon.MiniRoundTwoHandlerStub{},
		MiniRoundThreeHandler: mr3Handler,
		BlockFinalizer:        blockFinalizer.NewFinalizeBlockComponent(),
	})
}

// createFullRoundHandler builds a handler with both MR2 and MR3 stubs, used for
// MR2→MR3 transition tests where both handlers must be in place.
func createFullRoundHandler(
	selfID string,
	currentStep data.Step,
	currentRoundKey data.RoundKey,
	mr3Handler *testscommon.MiniRoundThreeHandlerStub,
) *roundHandler {
	return NewRoundHandler(RoundHandlerArgs{
		SelfID:                selfID,
		CurrentStep:           currentStep,
		CurrentRoundKey:       currentRoundKey,
		MiniRoundOneHandler:   &testscommon.MiniRoundOneHandlerStub{},
		MiniRoundTwoHandler:   &testscommon.MiniRoundTwoHandlerStub{},
		MiniRoundThreeHandler: mr3Handler,
		BlockFinalizer:        blockFinalizer.NewFinalizeBlockComponent(),
	})
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
