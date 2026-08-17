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
			Header: data.ChainBlockHeader{HeaderHash: []byte("block-hash")},
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
		firstBlock := &data.BlockOnChain{Header: data.ChainBlockHeader{HeaderHash: []byte("first")}}
		secondBlock := &data.BlockOnChain{Header: data.ChainBlockHeader{HeaderHash: []byte("second")}}

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

	t.Run("should store and retrieve mini-round three finalized block per round key", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}
		block := &data.BlockOnChain{
			FinalAnswers: []data.FinalAnswer{
				{TxHash: []byte("tx-1"), Answer: "synthesized", Status: data.FinalAnswerStatusSynthesized},
			},
		}

		err := component.FinalizeBlockMRThree(roundKey, block)

		require.NoError(t, err)
		finalizedBlock, err := component.GetFinalizedBlockInMRThree(roundKey)
		require.NoError(t, err)
		require.Same(t, block, finalizedBlock)
	})

	t.Run("MR3 finalized block does not collide with MR1 or MR2 blocks", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()
		key := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}
		mr3Block := &data.BlockOnChain{FinalAnswers: []data.FinalAnswer{{TxHash: []byte("tx-mr3")}}}
		mr1Block := &data.BlockOnChain{Header: data.ChainBlockHeader{HeaderHash: []byte("mr1")}}

		require.NoError(t, component.FinalizeBlockMRThree(key, mr3Block))
		require.NoError(t, component.FinalizeBlockMROne(data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}, mr1Block))

		retrieved, err := component.GetFinalizedBlockInMRThree(key)
		require.NoError(t, err)
		require.Same(t, mr3Block, retrieved)

		_, err = component.GetFinalizedBlockInMRTwo(key)
		require.Equal(t, ErrFinalizedBlockNotFound, err)
	})

	t.Run("should return ErrFinalizedBlockNotFound for MR3 when none stored", func(t *testing.T) {
		t.Parallel()

		component := NewFinalizeBlockComponent()

		finalizedBlock, err := component.GetFinalizedBlockInMRThree(data.RoundKey{})

		require.Nil(t, finalizedBlock)
		require.Equal(t, ErrFinalizedBlockNotFound, err)
	})
}
