package signing

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSigner_SignPromptExecutionHash(t *testing.T) {
	t.Parallel()

	t.Run("should sign prompt execution hash", func(t *testing.T) {
		t.Parallel()

		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer := NewSigner("validator-1", privateKey)
		executionResultHash := []byte("executed-prompts-block-hash")

		signature, err := signer.SignPromptExecutionHash(executionResultHash)

		require.NoError(t, err)
		require.NotEmpty(t, signature)
		require.NoError(t, signer.Verify(publicKey, executionResultHash, signature))
	})

	t.Run("should return ErrNilMessage when execution result hash is nil", func(t *testing.T) {
		t.Parallel()

		_, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)

		signer := NewSigner("validator-1", privateKey)

		signature, err := signer.SignPromptExecutionHash(nil)

		require.Nil(t, signature)
		require.Equal(t, ErrNilMessage, err)
	})

	t.Run("should return ErrNilPrivateKey when private key is nil", func(t *testing.T) {
		t.Parallel()

		signer := NewSigner("validator-1", nil)
		executionResultHash := []byte("executed-prompts-block-hash")

		signature, err := signer.SignPromptExecutionHash(executionResultHash)

		require.Nil(t, signature)
		require.Equal(t, ErrNilPrivateKey, err)
	})
}
