package miniround2

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

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
		signature, err := signer.SignPromptExecutionHash([]byte("different-execution-result-hash"))
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

	t.Run("should verify and store executed prompts message", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		roundState := state.NewRoundState()
		roundKey := createTestRoundKey()
		broadcaster := &testscommon.BroadcasterStub{}
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

		require.NoError(t, err)
		storedMessages, err := roundState.GetExecutedPromptsMessages(roundKey)
		require.NoError(t, err)
		require.Equal(t, []*data.AnswersBlockMessage{message}, storedMessages)
		require.NotNil(t, broadcaster.BroadcastAggregatedExecutionResultsMessage)
		require.Equal(t, data.AggregatedExecutionResultsConsensusMessage, broadcaster.BroadcastAggregatedExecutionResultsMessage.ConsensusMessageType)
		require.Equal(t, "leader", broadcaster.BroadcastAggregatedExecutionResultsMyID)
		require.Equal(t, []string{"leader", "validator-1", "validator-2"}, broadcaster.BroadcastAggregatedExecutionResultsTargets)

		aggregatedExecutionResults := broadcaster.BroadcastAggregatedExecutionResultsMessage.AggregatedExecutionResults
		require.NotNil(t, aggregatedExecutionResults)
		require.Equal(t, []string{"validator-1"}, aggregatedExecutionResults.Signers)
		require.Equal(t, [][]byte{message.BlockHash}, aggregatedExecutionResults.BlockHashes)
		require.Equal(t, [][]byte{message.BlockSignature}, aggregatedExecutionResults.BlockSignatures)
		require.Len(t, aggregatedExecutionResults.TxResults, 2)

		finalizedBlockInMRTwo, err := finalizer.GetFinalizedBlockInMRTwo(roundKey)
		require.NoError(t, err)
		require.Equal(t, finalizedBlock.Block, finalizedBlockInMRTwo.Block)
		require.Equal(t, finalizedBlock.SubdomainsFrequencies, finalizedBlockInMRTwo.SubdomainsFrequencies)
		require.Same(t, aggregatedExecutionResults, finalizedBlockInMRTwo.AggregatedExecutionResults)
	})

	t.Run("should not finalize aggregated execution results when broadcast fails", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("broadcast error")
		publicKey, privateKey := createTestKeyPair(t)
		finalizedBlock := createTestFinalizedBlock()
		signer := signing.NewSigner("validator-1", privateKey)
		message := createSignedExecutedPromptsMessage(t, signer, finalizedBlock)
		roundState := state.NewRoundState()
		roundKey := createTestRoundKey()
		broadcaster := &testscommon.BroadcasterStub{
			BroadcastAggregatedExecutionResultsErr: expectedErr,
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
		require.NotNil(t, broadcaster.BroadcastAggregatedExecutionResultsMessage)
		finalizedBlockInMRTwo, err := finalizer.GetFinalizedBlockInMRTwo(roundKey)
		require.Nil(t, finalizedBlockInMRTwo)
		require.Equal(t, blockFinalizer.ErrFinalizedBlockNotFound, err)
	})
}

func TestMiniRoundTwoHandler_createAggregatedExecutionResultsMessage(t *testing.T) {
	t.Parallel()

	t.Run("should aggregate answers deterministically by signer and transaction hash", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID: "leader",
		})
		roundKey := createTestRoundKey()
		messageFromSecondSigner := createExecutedPromptsMessageForAggregation("validator-b", "b")
		messageFromFirstSigner := createExecutedPromptsMessageForAggregation("validator-a", "a")

		aggregatedExecutionResults, err := handler.createAggregatedExecutionResultsMessage(roundKey, []*data.AnswersBlockMessage{
			messageFromSecondSigner,
			messageFromFirstSigner,
		})

		require.NoError(t, err)
		require.Equal(t, roundKey.Epoch, aggregatedExecutionResults.Epoch)
		require.Equal(t, roundKey.Round, aggregatedExecutionResults.Round)
		require.Equal(t, roundKey.MiniRound, aggregatedExecutionResults.MiniRound)
		require.Equal(t, "leader", aggregatedExecutionResults.SenderID)
		require.Equal(t, []byte("canonical-mr1-header-hash"), aggregatedExecutionResults.CanonicalBlockHash)
		require.Equal(t, []string{"validator-a", "validator-b"}, aggregatedExecutionResults.Signers)
		require.Equal(t, [][]byte{[]byte("block-hash-a"), []byte("block-hash-b")}, aggregatedExecutionResults.BlockHashes)
		require.Equal(t, [][]byte{[]byte("signature-a"), []byte("signature-b")}, aggregatedExecutionResults.BlockSignatures)
		require.Len(t, aggregatedExecutionResults.TxResults, 2)

		require.Equal(t, []byte("txHash1"), aggregatedExecutionResults.TxResults[0].TxHash)
		require.Equal(t, []string{"answer-a-1", "answer-b-1"}, extractAnswers(aggregatedExecutionResults.TxResults[0].Answers))

		require.Equal(t, []byte("txHash2"), aggregatedExecutionResults.TxResults[1].TxHash)
		require.Equal(t, []string{"answer-a-2", "answer-b-2"}, extractAnswers(aggregatedExecutionResults.TxResults[1].Answers))
	})

	t.Run("should return ErrExecutedPromptsAnswersMismatch when a signer misses a transaction answer", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID: "leader",
		})
		firstMessage := createExecutedPromptsMessageForAggregation("validator-a", "a")
		secondMessage := createExecutedPromptsMessageForAggregation("validator-b", "b")
		delete(secondMessage.Answers, "txHash2")

		_, err := handler.createAggregatedExecutionResultsMessage(createTestRoundKey(), []*data.AnswersBlockMessage{
			firstMessage,
			secondMessage,
		})

		require.Equal(t, ErrExecutedPromptsAnswersMismatch, err)
	})

	t.Run("should return ErrCanonicalBlockHashMismatch when messages target different canonical blocks", func(t *testing.T) {
		t.Parallel()

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			myID: "leader",
		})
		firstMessage := createExecutedPromptsMessageForAggregation("validator-a", "a")
		secondMessage := createExecutedPromptsMessageForAggregation("validator-b", "b")
		secondMessage.CanonicalBlockHash = []byte("different-canonical-hash")

		_, err := handler.createAggregatedExecutionResultsMessage(createTestRoundKey(), []*data.AnswersBlockMessage{
			firstMessage,
			secondMessage,
		})

		require.Equal(t, ErrCanonicalBlockHashMismatch, err)
	})
}

type testMiniRoundTwoHandlerArgs struct {
	myID              string
	signer            signing.MessageSigner
	roundState        state.RoundState
	broadcaster       *testscommon.BroadcasterStub
	blockFinalizer    blockFinalizer.BlockFinalizer
	validatorRegistry validators.ValidatorRegistry
}

func createTestMiniRoundTwoHandler(args testMiniRoundTwoHandlerArgs) *miniRoundTwoHandler {
	return &miniRoundTwoHandler{
		myID:              args.myID,
		signer:            args.signer,
		roundState:        args.roundState,
		broadcaster:       args.broadcaster,
		blockFinalizer:    args.blockFinalizer,
		validatorRegistry: args.validatorRegistry,
		logger:            slog.Default(),
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
	tx2 := &testscommon.TransactionStub{}
	tx2.SetTxHash([]byte("txHash2"))

	return &data.BlockOnChain{
		Block: data.Block{
			Header: data.BlockHeader{
				HeaderHash: []byte("canonical-mr1-header-hash"),
			},
			Body: data.BlockBody{
				Transactions: []data.Transaction{tx1, tx2},
			},
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

	signature, err := signer.SignPromptExecutionHash(executionResultHash)
	require.NoError(t, err)

	return &data.AnswersBlockMessage{
		Epoch:              1,
		Round:              2,
		MiniRound:          uint64(data.MiniRoundTwo),
		SenderID:           "validator-1",
		CanonicalBlockHash: finalizedBlock.Block.Header.HeaderHash,
		Answers: data.AnswersTxMessage{
			"txHash1": executionResult.TxsResults[0],
			"txHash2": executionResult.TxsResults[1],
		},
		BlockHash:      executionResultHash,
		BlockSignature: signature,
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

func extractAnswers(txResults []data.TransactionResult) []string {
	answers := make([]string, 0, len(txResults))
	for _, txResult := range txResults {
		answers = append(answers, txResult.Answer)
	}

	return answers
}
