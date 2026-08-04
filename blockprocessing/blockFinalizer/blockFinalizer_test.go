package blockFinalizer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestFinalizeBlockComponent(t *testing.T) {
	t.Parallel()

	t.Run("should store and retrieve mini-round one finalized block per round key", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
		block := &data.BlockOnChain{
			Block: data.Block{
				Header: data.BlockHeader{HeaderHash: []byte("block-hash")},
			},
		}

		err := component.FinalizeBlockMROne(roundKey, block)

		require.NoError(t, err)
		finalizedBlock, err := component.GetFinalizedBlockInMROne(roundKey)
		require.NoError(t, err)
		require.Same(t, block, finalizedBlock)
	})

	t.Run("should store and retrieve mini-round two finalized block per round key", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		block := &data.BlockOnChain{
			AggregatedExecutionResults: data.AggregatedExecutionResults{
				{
					TxHash: []byte("txHash1"),
				},
			},
		}

		err := component.FinalizeBlockMRTwo(roundKey, block)

		require.NoError(t, err)
		finalizedBlock, err := component.GetFinalizedBlockInMRTwo(roundKey)
		require.NoError(t, err)
		require.Same(t, block, finalizedBlock)
	})

	t.Run("should keep finalized blocks isolated by round key", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()
		firstRoundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
		secondRoundKey := data.RoundKey{Epoch: 1, Round: 3, MiniRound: uint64(data.MiniRoundOne)}
		firstBlock := &data.BlockOnChain{Block: data.Block{Header: data.BlockHeader{HeaderHash: []byte("first")}}}
		secondBlock := &data.BlockOnChain{Block: data.Block{Header: data.BlockHeader{HeaderHash: []byte("second")}}}

		require.NoError(t, component.FinalizeBlockMROne(firstRoundKey, firstBlock))
		require.NoError(t, component.FinalizeBlockMROne(secondRoundKey, secondBlock))

		finalizedFirstBlock, err := component.GetFinalizedBlockInMROne(firstRoundKey)
		require.NoError(t, err)
		require.Same(t, firstBlock, finalizedFirstBlock)

		finalizedSecondBlock, err := component.GetFinalizedBlockInMROne(secondRoundKey)
		require.NoError(t, err)
		require.Same(t, secondBlock, finalizedSecondBlock)
	})

	t.Run("should return ErrNilFinalizedBlock when finalized block is nil", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()

		err := component.FinalizeBlockMROne(data.RoundKey{}, nil)

		require.Equal(t, ErrNilFinalizedBlock, err)
	})

	t.Run("should return ErrFinalizedBlockNotFound when finalized block does not exist", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()

		finalizedBlock, err := component.GetFinalizedBlockInMROne(data.RoundKey{})

		require.Nil(t, finalizedBlock)
		require.Equal(t, ErrFinalizedBlockNotFound, err)
	})
}
