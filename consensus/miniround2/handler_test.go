package miniround2

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

func TestMiniRoundTwoHandler_verifyExecutePromptsMessage(t *testing.T) {
	t.Parallel()

	t.Run("should verify executed prompts message", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		signature := message.BlockSignature
		executionResultHash := message.BlockHash
		require.NotEmpty(t, signature)

		expectedHash, err := hashing.ComputePromptExecutionHash(createTestExecutionResult())
		require.NoError(t, err)
		require.Equal(t, expectedHash, executionResultHash)

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer:         signer,
			blockFinalizer: createSeededFinalizer(t, finalizedBlock),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
			},
		})

		err = handler.verifyExecutePromptsMessage(createTestRoundKey(), message)

		require.NoError(t, err)
	})

	t.Run("should return ErrSignerIsNotValidator when signer is not registered", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer: signing.NewSigner("leader", nil),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{},
			},
		})

		err := handler.verifyExecutePromptsMessage(data.RoundKey{}, &data.AnswersBlockMessage{
			SenderID: "validator-1",
		})

		require.Equal(t, ErrSignerIsNotValidator, err)
	})

	t.Run("should return ErrValidatorNotPartOfConsensusGroup when signer is outside consensus group", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer: signing.NewSigner("leader", nil),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{},
			},
		})

		err := handler.verifyExecutePromptsMessage(data.RoundKey{}, &data.AnswersBlockMessage{
			SenderID: "validator-1",
		})

		require.Equal(t, ErrValidatorNotPartOfConsensusGroup, err)
	})

	t.Run("should return public key lookup error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("public key lookup error")
		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer: signing.NewSigner("leader", nil),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				GetPublicKeyErr: expectedErr,
			},
		})

		err := handler.verifyExecutePromptsMessage(data.RoundKey{}, &data.AnswersBlockMessage{
			SenderID: "validator-1",
		})

		require.Equal(t, expectedErr, err)
	})

	t.Run("should return signature verification error", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		signature, err := signer.Sign([]byte("different-execution-result-hash"))
		require.NoError(t, err)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		message.BlockSignature = signature

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer:         signer,
			blockFinalizer: createSeededFinalizer(t, finalizedBlock),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
			},
		})

		err = handler.verifyExecutePromptsMessage(createTestRoundKey(), message)

		require.Equal(t, signing.ErrWrongSignature, err)
	})

	t.Run("should return ErrExecutionResultHashMismatch when answers do not match block hash", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		message.Answers["txHash1"] = data.TransactionResult{
			TxHash:            []byte("txHash1"),
			Answer:            "tampered answer",
			ActualConsumption: 3,
		}

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer:         signer,
			blockFinalizer: createSeededFinalizer(t, finalizedBlock),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
			},
		})

		err := handler.verifyExecutePromptsMessage(createTestRoundKey(), message)

		require.Equal(t, ErrExecutionResultHashMismatch, err)
	})
}

func TestMiniRoundTwoHandler_HandleExecutedPromptsMessage(t *testing.T) {
	t.Parallel()

	t.Run("should complete classification flow for a single-validator committee", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		roundState := state.NewRoundState()
		roundKey := createTestRoundKey()
		broadcaster := &testscommon.BroadcasterStub{}
		finalizer := createSeededFinalizer(t, finalizedBlock)
		selfInbox := make(chan data.RoundEvent, 32)
		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID:           "validator-1",
			signer:         signer,
			answerJudge:    singleCandidateAnswerJudge{},
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: finalizer,
			selfInbox:      selfInbox,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "validator-1",
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
				ConsensusGroupSizeValue: 1,
				ConsensusGroupValue:     []string{"validator-1"},
				ValidatorsIDs:           []string{"validator-1"},
			},
		})
		t.Cleanup(func() { handler.judgeWg.Wait() })

		err := handler.HandleExecutedPromptsMessage(roundKey, message)

		require.NoError(t, err)
		storedMessages, err := roundState.GetExecutedPromptsMessages(roundKey)
		require.NoError(t, err)
		require.Equal(t, []*data.AnswersBlockMessage{message}, storedMessages)
		require.NotNil(t, broadcaster.BroadcastAnswerEvidenceMessage)
		require.Equal(t, data.AnswerEvidenceConsensusMessage, broadcaster.BroadcastAnswerEvidenceMessage.ConsensusMessageType)
		require.Equal(t, "validator-1", broadcaster.BroadcastAnswerEvidenceMyID)
		require.Equal(t, []string{"validator-1"}, broadcaster.BroadcastAnswerEvidenceTargets)

		answerEvidence := broadcaster.BroadcastAnswerEvidenceMessage.AnswerEvidence
		require.NotNil(t, answerEvidence)
		require.Equal(t, []string{"validator-1"}, answerEvidence.Signers)
		require.Equal(t, [][]byte{message.BlockHash}, answerEvidence.BlockHashes)
		require.Equal(t, [][]byte{message.BlockSignature}, answerEvidence.BlockSignatures)
		require.Equal(t, []data.AnswersTxMessage{message.Answers}, answerEvidence.Answers)

		// The judge runs asynchronously. Wait for it then process the leader's own
		// vote through the handler (simulating the event loop draining selfInbox).
		handler.judgeWg.Wait()
		event := <-selfInbox
		require.Equal(t, data.ConsensusMessageEvent, event.Type)
		require.Equal(t, data.AnswerClassificationVoteConsensusMessage, event.Message.ConsensusMessageType)
		require.NoError(t, handler.HandleAnswerClassificationVote(roundKey, event.Message.AnswerClassificationVote))

		finalizedBlockInMRTwo, err := finalizer.GetFinalizedBlockInMRTwo(roundKey)
		require.NoError(t, err)
		require.Equal(t, finalizedBlock.Header, finalizedBlockInMRTwo.Header)
		require.Equal(t, finalizedBlock.Body, finalizedBlockInMRTwo.Body)
		require.Equal(t, finalizedBlock.SubdomainsFrequencies, finalizedBlockInMRTwo.SubdomainsFrequencies)
		require.Equal(t, data.AggregatedExecutionResults{
			{
				TxHash: []byte("txHash1"),
				Answers: []data.TransactionResult{
					message.Answers["txHash1"],
				},
			},
			{
				TxHash: []byte("txHash2"),
				Answers: []data.TransactionResult{
					message.Answers["txHash2"],
				},
			},
		}, finalizedBlockInMRTwo.AggregatedExecutionResults)
		require.Same(t, answerEvidence, finalizedBlockInMRTwo.AnswerEvidence)
		require.Len(t, finalizedBlockInMRTwo.AnswerClassifications, 2)
	})

	t.Run("should not finalize when answer-evidence broadcast fails", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("broadcast error")
		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		roundState := state.NewRoundState()
		roundKey := createTestRoundKey()
		broadcaster := &testscommon.BroadcasterStub{
			BroadcastAnswerEvidenceErr: expectedErr,
		}
		finalizer := createSeededFinalizer(t, finalizedBlock)
		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID:           "leader",
			signer:         signer,
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: finalizer,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
				ValidatorsIDs: []string{"leader", "validator-1", "validator-2"},
			},
		})

		err := handler.HandleExecutedPromptsMessage(roundKey, message)

		require.Equal(t, expectedErr, err)
		require.NotNil(t, broadcaster.BroadcastAnswerEvidenceMessage)
		finalizedBlockInMRTwo, err := finalizer.GetFinalizedBlockInMRTwo(roundKey)
		require.Nil(t, finalizedBlockInMRTwo)
		require.Equal(t, blockFinalizer.ErrFinalizedBlockNotFound, err)
	})
}

type singleCandidateAnswerJudge struct{}

func (singleCandidateAnswerJudge) JudgeTransactionAnswers(agent.AnswerJudgeRequest) (string, error) {
	return `{"classifications":[{"candidateId":"candidate-1","category":"CORRECT"}]}`, nil
}

func TestMiniRoundTwoHandler_verifyAnswerEvidence(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrMessageNotFromLeader when message sender is not selected leader", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
			},
		})
		message := &data.AggregatedExecutionResultsMessage{
			SenderID: "validator-1",
		}

		err := handler.verifyAnswerEvidence(createTestRoundKey(), message)

		require.Equal(t, ErrMessageNotFromLeader, err)
	})

	t.Run("should return ErrAnswerEvidenceMismatch when certificate arrays are not aligned", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
			},
		})
		roundKey := createTestRoundKey()
		message := &data.AggregatedExecutionResultsMessage{
			Epoch:              roundKey.Epoch,
			Round:              roundKey.Round,
			MiniRound:          roundKey.MiniRound,
			SenderID:           "leader",
			CanonicalBlockHash: []byte("canonical-mr1-header-hash"),
			Signers:            []string{"validator-1"},
			BlockHashes:        [][]byte{[]byte("block-hash")},
			BlockSignatures:    nil,
			Answers:            []data.AnswersTxMessage{{}},
		}

		err := handler.verifyAnswerEvidence(roundKey, message)

		require.Equal(t, ErrAnswerEvidenceMismatch, err)
	})

	t.Run("should return signature verification error when a signature is invalid", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		executedPromptsMessage := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		answerEvidence := createAnswerEvidenceFromExecutedPrompts("leader", createTestRoundKey(), executedPromptsMessage)
		answerEvidence.BlockSignatures[0] = []byte("invalid-signature")
		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer:         signing.NewSigner("validator-2", nil),
			blockFinalizer: createSeededFinalizer(t, finalizedBlock),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
				RegisteredValidators: map[string]bool{
					"validator-1": true,
				},
				ConsensusValidators: map[string]bool{
					"validator-1": true,
				},
				PublicKeysByValidatorID: map[string][]byte{
					"validator-1": publicKey,
				},
			},
		})

		err := handler.verifyAnswerEvidence(createTestRoundKey(), answerEvidence)

		require.Equal(t, signing.ErrWrongSignature, err)
	})
}

func TestMiniRoundTwoHandler_createFinalizedAggregatedExecutionResults(t *testing.T) {
	t.Parallel()

	t.Run("should create deterministic transaction answer groups", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{})
		firstMessage := createExecutedPromptsMessageForAggregation("validator-a", "a")
		secondMessage := createExecutedPromptsMessageForAggregation("validator-b", "b")
		answerEvidence := &data.AggregatedExecutionResultsMessage{
			Answers: []data.AnswersTxMessage{
				firstMessage.Answers,
				secondMessage.Answers,
			},
		}

		aggregatedExecutionResults, err := handler.createFinalizedAggregatedExecutionResults(answerEvidence)

		require.NoError(t, err)
		require.Equal(t, data.AggregatedExecutionResults{
			{
				TxHash: []byte("txHash1"),
				Answers: []data.TransactionResult{
					firstMessage.Answers["txHash1"],
					secondMessage.Answers["txHash1"],
				},
			},
			{
				TxHash: []byte("txHash2"),
				Answers: []data.TransactionResult{
					firstMessage.Answers["txHash2"],
					secondMessage.Answers["txHash2"],
				},
			},
		}, aggregatedExecutionResults)
	})

	t.Run("should return ErrExecutedPromptsAnswersMismatch when answer sets have different transactions", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{})
		firstMessage := createExecutedPromptsMessageForAggregation("validator-a", "a")
		secondMessage := createExecutedPromptsMessageForAggregation("validator-b", "b")
		delete(secondMessage.Answers, "txHash2")
		answerEvidence := &data.AggregatedExecutionResultsMessage{
			Answers: []data.AnswersTxMessage{
				firstMessage.Answers,
				secondMessage.Answers,
			},
		}

		aggregatedExecutionResults, err := handler.createFinalizedAggregatedExecutionResults(answerEvidence)

		require.Nil(t, aggregatedExecutionResults)
		require.Equal(t, ErrExecutedPromptsAnswersMismatch, err)
	})

	t.Run("should return ErrNilAnswers when message has no answers", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{})

		aggregatedExecutionResults, err := handler.createFinalizedAggregatedExecutionResults(&data.AggregatedExecutionResultsMessage{})

		require.Nil(t, aggregatedExecutionResults)
		require.Equal(t, ErrNilAnswers, err)
	})
}

func TestMiniRoundTwoHandler_createAnswerEvidence(t *testing.T) {
	t.Parallel()

	t.Run("should build answer evidence deterministically by signer", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID: "leader",
		})
		roundKey := createTestRoundKey()
		messageFromSecondSigner := createExecutedPromptsMessageForAggregation("validator-b", "b")
		messageFromFirstSigner := createExecutedPromptsMessageForAggregation("validator-a", "a")

		answerEvidence, err := handler.createAnswerEvidence(roundKey, []*data.AnswersBlockMessage{
			messageFromSecondSigner,
			messageFromFirstSigner,
		})

		require.NoError(t, err)
		require.Equal(t, roundKey.Epoch, answerEvidence.Epoch)
		require.Equal(t, roundKey.Round, answerEvidence.Round)
		require.Equal(t, roundKey.MiniRound, answerEvidence.MiniRound)
		require.Equal(t, "leader", answerEvidence.SenderID)
		require.Equal(t, []byte("canonical-mr1-header-hash"), answerEvidence.CanonicalBlockHash)
		require.Equal(t, []string{"validator-a", "validator-b"}, answerEvidence.Signers)
		require.Equal(t, [][]byte{[]byte("block-hash-a"), []byte("block-hash-b")}, answerEvidence.BlockHashes)
		require.Equal(t, [][]byte{[]byte("signature-a"), []byte("signature-b")}, answerEvidence.BlockSignatures)
		require.Equal(t, []data.AnswersTxMessage{
			messageFromFirstSigner.Answers,
			messageFromSecondSigner.Answers,
		}, answerEvidence.Answers)
	})

	t.Run("should return ErrCanonicalBlockHashMismatch when messages target different canonical blocks", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID: "leader",
		})
		firstMessage := createExecutedPromptsMessageForAggregation("validator-a", "a")
		secondMessage := createExecutedPromptsMessageForAggregation("validator-b", "b")
		secondMessage.CanonicalBlockHash = []byte("different-canonical-hash")

		_, err := handler.createAnswerEvidence(createTestRoundKey(), []*data.AnswersBlockMessage{
			firstMessage,
			secondMessage,
		})

		require.Equal(t, ErrCanonicalBlockHashMismatch, err)
	})
}

type testMiniRoundTwoHandlerArgs struct {
	myID               string
	signer             signing.MessageSigner
	answerJudge        agent.AnswersJudge
	judgeModelMetadata string
	roundState         state.RoundState
	broadcaster        *testscommon.BroadcasterStub
	blockFinalizer     blockFinalizer.BlockFinalizer
	validatorRegistry  validators.ValidatorRegistry
	selfInbox          chan data.RoundEvent
}

func createTestMiniRoundTwoHandler(args testMiniRoundTwoHandlerArgs) *miniRoundTwoHandler {
	return &miniRoundTwoHandler{
		myID:               args.myID,
		signer:             args.signer,
		answerJudge:        args.answerJudge,
		judgeModelMetadata: args.judgeModelMetadata,
		roundState:         args.roundState,
		broadcaster:        args.broadcaster,
		blockFinalizer:     args.blockFinalizer,
		validatorRegistry:  args.validatorRegistry,
		logger:             slog.Default(),
		selfInbox:          (chan<- data.RoundEvent)(args.selfInbox),
	}
}

func createTestKeyPair(t *testing.T) ([]byte, []byte) {
	t.Helper()

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	return publicKey, privateKey
}

func createTestRoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
}

func createTestMiniRoundOneRoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
}

func createSeededFinalizer(t *testing.T, finalizedBlock *data.BlockOnChain) *blockFinalizer.FinalizeBlockComponent {
	t.Helper()

	finalizer := blockFinalizer.NewFinalizeBlockComponent()
	err := finalizer.FinalizeBlockMROne(createTestMiniRoundOneRoundKey(), finalizedBlock)
	require.NoError(t, err)

	return finalizer
}

func createTestFinalizedBlock() *data.BlockOnChain {
	tx1 := &testscommon.TransactionStub{}
	tx1.SetTxHash([]byte("txHash1"))
	tx1.SetPrompt([]byte("prompt one"))
	tx2 := &testscommon.TransactionStub{}
	tx2.SetTxHash([]byte("txHash2"))
	tx2.SetPrompt([]byte("prompt two"))

	return &data.BlockOnChain{
		Header: data.ChainBlockHeader{
			HeaderHash: []byte("canonical-mr1-header-hash"),
		},
		Body: data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		},
		SubdomainsFrequencies: data.SubdomainsFrequency{
			"ml_ai_engineering": 3,
		},
	}
}

func createTestExecutionResult() *data.BlockBodyExecutionResultMRTwo {
	return &data.BlockBodyExecutionResultMRTwo{
		TxsResults: []data.TransactionResult{
			{
				TxHash:            []byte("txHash1"),
				Answer:            "answer one",
				ActualConsumption: 3,
			},
			{
				TxHash:            []byte("txHash2"),
				Answer:            "answer two",
				ActualConsumption: 5,
			},
		},
		TotalConsumption: 8,
	}
}

func createSignedExecutedPromptsMessage(
	t *testing.T,
	signer signing.MessageSigner,
	finalizedBlock *data.BlockOnChain,
) *data.AnswersBlockMessage {
	t.Helper()

	executionResult := createTestExecutionResult()
	executionResultHash, err := hashing.ComputePromptExecutionHash(executionResult)
	require.NoError(t, err)

	signature, err := signer.Sign(executionResultHash)
	require.NoError(t, err)

	return &data.AnswersBlockMessage{
		Epoch:              1,
		Round:              2,
		MiniRound:          uint64(data.MiniRoundTwo),
		SenderID:           signer.ID(),
		CanonicalBlockHash: finalizedBlock.Header.HeaderHash,
		Answers: data.AnswersTxMessage{
			"txHash1": executionResult.TxsResults[0],
			"txHash2": executionResult.TxsResults[1],
		},
		BlockHash:      executionResultHash,
		BlockSignature: signature,
	}
}

func createAnswerEvidenceFromExecutedPrompts(
	leaderID string,
	roundKey data.RoundKey,
	messages ...*data.AnswersBlockMessage,
) *data.AggregatedExecutionResultsMessage {
	signers := make([]string, 0, len(messages))
	blockHashes := make([][]byte, 0, len(messages))
	blockSignatures := make([][]byte, 0, len(messages))
	answers := make([]data.AnswersTxMessage, 0, len(messages))
	canonicalBlockHash := []byte(nil)
	if len(messages) > 0 {
		canonicalBlockHash = messages[0].CanonicalBlockHash
	}

	for _, message := range messages {
		signers = append(signers, message.SenderID)
		blockHashes = append(blockHashes, message.BlockHash)
		blockSignatures = append(blockSignatures, message.BlockSignature)
		answers = append(answers, message.Answers)
	}

	return &data.AggregatedExecutionResultsMessage{
		Epoch:              roundKey.Epoch,
		Round:              roundKey.Round,
		MiniRound:          roundKey.MiniRound,
		SenderID:           leaderID,
		CanonicalBlockHash: canonicalBlockHash,
		Signers:            signers,
		BlockHashes:        blockHashes,
		BlockSignatures:    blockSignatures,
		Answers:            answers,
	}
}

func createExecutedPromptsMessageForAggregation(senderID string, answerSuffix string) *data.AnswersBlockMessage {
	return &data.AnswersBlockMessage{
		Epoch:              1,
		Round:              2,
		MiniRound:          uint64(data.MiniRoundTwo),
		SenderID:           senderID,
		CanonicalBlockHash: []byte("canonical-mr1-header-hash"),
		Answers: data.AnswersTxMessage{
			"txHash2": {
				TxHash:            []byte("txHash2"),
				Answer:            "answer-" + answerSuffix + "-2",
				ActualConsumption: 5,
			},
			"txHash1": {
				TxHash:            []byte("txHash1"),
				Answer:            "answer-" + answerSuffix + "-1",
				ActualConsumption: 3,
			},
		},
		BlockHash:      []byte("block-hash-" + answerSuffix),
		BlockSignature: []byte("signature-" + answerSuffix),
	}
}
