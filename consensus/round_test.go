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

	t.Run("should execute block and await aggregated execution results when local node is not leader", func(t *testing.T) {
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

	t.Run("should route aggregated execution results to mini-round two handler and finish round", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		aggregatedExecutionResults := &data.AggregatedExecutionResultsMessage{
			Epoch:     roundKey.Epoch,
			Round:     roundKey.Round,
			MiniRound: roundKey.MiniRound,
			SenderID:  "validator-1",
		}
		miniRoundTwoHandler := &testscommon.MiniRoundTwoHandlerStub{}
		handler := createTestRoundHandler("validator-2", data.StepAwaitAggregatedExecutionResults, roundKey, miniRoundTwoHandler)

		err := handler.HandleMessage(data.ConsensusMessage{
			ConsensusMessageType:       data.AggregatedExecutionResultsConsensusMessage,
			AggregatedExecutionResults: aggregatedExecutionResults,
		})

		require.NoError(t, err)
		require.True(t, miniRoundTwoHandler.HandleAggregatedExecutionResultsCalled)
		require.Equal(t, roundKey, miniRoundTwoHandler.HandleAggregatedExecutionResultsKey)
		require.Same(t, aggregatedExecutionResults, miniRoundTwoHandler.HandleAggregatedExecutionResultsMessage)
		require.Equal(t, data.StepFinished, handler.currentStep)
	})

	t.Run("should reject executed prompts when not collecting execution results", func(t *testing.T) {
		t.Parallel()

		roundKey := createMiniRoundTwoRoundKey()
		handler := createTestRoundHandler("validator-1", data.StepAwaitAggregatedExecutionResults, roundKey, &testscommon.MiniRoundTwoHandlerStub{})

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
