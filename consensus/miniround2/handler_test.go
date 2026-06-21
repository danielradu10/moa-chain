package miniround2

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

func TestMiniRoundTwoHandler_verifyExecutePromptsMessage(t *testing.T) {
	t.Parallel()

	t.Run("should verify executed prompts message", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey := createTestKeyPair(t)
		executionResultHash := []byte("execution-result-hash")
		signer := signing.NewSigner("validator-1", privateKey)
		signature, err := signer.SignPromptExecutionHash(executionResultHash)
		require.NoError(t, err)

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer: signer,
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

		err = handler.verifyExecutePromptsMessage(data.RoundKey{}, &data.AnswersBlockMessage{
			SenderID:       "validator-1",
			BlockHash:      executionResultHash,
			BlockSignature: signature,
		})

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
		signer := signing.NewSigner("validator-1", privateKey)
		signature, err := signer.SignPromptExecutionHash([]byte("different-execution-result-hash"))
		require.NoError(t, err)

		handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
			signer: signer,
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

		err = handler.verifyExecutePromptsMessage(data.RoundKey{}, &data.AnswersBlockMessage{
			SenderID:       "validator-1",
			BlockHash:      []byte("execution-result-hash"),
			BlockSignature: signature,
		})

		require.Equal(t, signing.ErrWrongSignature, err)
	})
}

type testMiniRoundTwoHandlerArgs struct {
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
}

func createTestMiniRoundTwoHandler(args testMiniRoundTwoHandlerArgs) *miniRoundTwoHandler {
	return &miniRoundTwoHandler{
		signer:            args.signer,
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
