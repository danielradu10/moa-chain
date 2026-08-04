package hashing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestComputePromptExecutionHash(t *testing.T) {
	t.Parallel()

	t.Run("should compute deterministic hash", func(t *testing.T) {
		t.Parallel()

		result := createPromptExecutionResult()

		hash1, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		hash2, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		require.Equal(t, hash1, hash2)
		require.Len(t, hash1, 32)
	})

	t.Run("should not include block hash in computed hash", func(t *testing.T) {
		t.Parallel()

		result := createPromptExecutionResult()
		resultWithBlockHash := createPromptExecutionResult()
		resultWithBlockHash.BlockHash = []byte("already computed hash")

		hash1, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		hash2, err := ComputePromptExecutionHash(resultWithBlockHash)
		require.NoError(t, err)

		require.Equal(t, hash1, hash2)
	})

	t.Run("should change hash when answer changes", func(t *testing.T) {
		t.Parallel()

		result := createPromptExecutionResult()
		resultWithDifferentAnswer := createPromptExecutionResult()
		resultWithDifferentAnswer.TxsResults[0].Answer = "different answer"

		hash1, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		hash2, err := ComputePromptExecutionHash(resultWithDifferentAnswer)
		require.NoError(t, err)

		require.NotEqual(t, hash1, hash2)
	})

	t.Run("should change hash when transaction order changes", func(t *testing.T) {
		t.Parallel()

		result := createPromptExecutionResult()
		resultWithDifferentOrder := createPromptExecutionResult()
		resultWithDifferentOrder.TxsResults[0], resultWithDifferentOrder.TxsResults[1] =
			resultWithDifferentOrder.TxsResults[1], resultWithDifferentOrder.TxsResults[0]

		hash1, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		hash2, err := ComputePromptExecutionHash(resultWithDifferentOrder)
		require.NoError(t, err)

		require.NotEqual(t, hash1, hash2)
	})

	t.Run("should change hash when total consumption changes", func(t *testing.T) {
		t.Parallel()

		result := createPromptExecutionResult()
		resultWithDifferentTotalConsumption := createPromptExecutionResult()
		resultWithDifferentTotalConsumption.TotalConsumption++

		hash1, err := ComputePromptExecutionHash(result)
		require.NoError(t, err)

		hash2, err := ComputePromptExecutionHash(resultWithDifferentTotalConsumption)
		require.NoError(t, err)

		require.NotEqual(t, hash1, hash2)
	})

	t.Run("should return error when execution result is nil", func(t *testing.T) {
		t.Parallel()

		hash, err := ComputePromptExecutionHash(nil)

		require.Nil(t, hash)
		require.Error(t, err)
	})
}

func createPromptExecutionResult() *data.BlockBodyExecutionResultMRTwo {
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
