package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/chain"
	"moa-chain/data"
)

func TestBlockchainState_CurrentBlockHeader(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrEmptyBlockchainState when chain is empty", func(t *testing.T) {
		t.Parallel()

		bs := NewBlockchainState(chain.NewChain())
		header, err := bs.CurrentBlockHeader()
		require.ErrorIs(t, err, ErrEmptyBlockchainState)
		require.Nil(t, header)
	})

	t.Run("returns header of the only appended block", func(t *testing.T) {
		t.Parallel()

		c := chain.NewChain()
		block := chainBlock(1, nil)
		require.NoError(t, c.Append(block))

		bs := NewBlockchainState(c)
		header, err := bs.CurrentBlockHeader()
		require.NoError(t, err)
		require.Equal(t, &block.Header, header)
	})

	t.Run("reflects new head after a second block is appended", func(t *testing.T) {
		t.Parallel()

		c := chain.NewChain()
		b1 := chainBlock(1, nil)
		require.NoError(t, c.Append(b1))
		b2 := chainBlock(2, b1.Header.HeaderHash)
		require.NoError(t, c.Append(b2))

		bs := NewBlockchainState(c)
		header, err := bs.CurrentBlockHeader()
		require.NoError(t, err)
		require.Equal(t, &b2.Header, header)
	})
}

func TestBlockchainState_Update(t *testing.T) {
	t.Parallel()

	t.Run("CurrentRound reflects last Update", func(t *testing.T) {
		t.Parallel()

		bs := NewBlockchainState(chain.NewChain()).(*blockchainState)
		bs.Update(7, data.MiniRoundTwo, 0)

		round, err := bs.CurrentRound()
		require.NoError(t, err)
		require.Equal(t, uint64(7), round)
	})

	t.Run("CurrentMiniRound reflects last Update", func(t *testing.T) {
		t.Parallel()

		bs := NewBlockchainState(chain.NewChain()).(*blockchainState)
		bs.Update(1, data.MiniRoundTwo, 0)

		mr, err := bs.CurrentMiniRound()
		require.NoError(t, err)
		require.Equal(t, uint64(data.MiniRoundTwo), mr)
	})

	t.Run("CurrentEpoch reflects last Update", func(t *testing.T) {
		t.Parallel()

		bs := NewBlockchainState(chain.NewChain()).(*blockchainState)
		bs.Update(1, data.MiniRoundOne, 3)

		epoch, err := bs.CurrentEpoch()
		require.NoError(t, err)
		require.Equal(t, uint64(3), epoch)
	})

	t.Run("successive Updates overwrite previous values", func(t *testing.T) {
		t.Parallel()

		bs := NewBlockchainState(chain.NewChain()).(*blockchainState)
		bs.Update(1, data.MiniRoundOne, 0)
		bs.Update(1, data.MiniRoundTwo, 0)
		bs.Update(1, data.MiniRoundThree, 0)

		mr, err := bs.CurrentMiniRound()
		require.NoError(t, err)
		require.Equal(t, uint64(data.MiniRoundThree), mr)
	})
}

func TestBlockchainState_CurrentBlockHeader_UpdatesWithChain(t *testing.T) {
	t.Parallel()

	t.Run("CurrentBlockHeader tracks chain appends without calling Update", func(t *testing.T) {
		t.Parallel()

		c := chain.NewChain()
		bs := NewBlockchainState(c)

		b1 := chainBlock(1, nil)
		require.NoError(t, c.Append(b1))

		header, err := bs.CurrentBlockHeader()
		require.NoError(t, err)
		require.Equal(t, &b1.Header, header)

		b2 := chainBlock(2, b1.Header.HeaderHash)
		require.NoError(t, c.Append(b2))

		header, err = bs.CurrentBlockHeader()
		require.NoError(t, err)
		require.Equal(t, &b2.Header, header)
	})
}

func chainBlock(round uint64, prevHash []byte) *data.BlockOnChain {
	return &data.BlockOnChain{
		Header: data.ChainBlockHeader{
			Round:        round,
			HeaderHash:   []byte{byte(round)},
			PreviousHash: prevHash,
		},
	}
}
