package chain

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestChain_Head(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrEmptyChain when no blocks have been appended", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		block, err := c.Head()
		require.ErrorIs(t, err, ErrEmptyChain)
		require.Nil(t, block)
	})

	t.Run("returns the only block after one append", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b := blockWithRound(1)
		require.NoError(t, c.Append(b))

		head, err := c.Head()
		require.NoError(t, err)
		require.Equal(t, b, head)
	})

	t.Run("returns the last appended block after multiple appends", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b1 := blockWithRound(1)
		require.NoError(t, c.Append(b1))
		b2 := blockLinked(2, b1.Header.HeaderHash)
		require.NoError(t, c.Append(b2))
		last := blockLinked(3, b2.Header.HeaderHash)
		require.NoError(t, c.Append(last))

		head, err := c.Head()
		require.NoError(t, err)
		require.Equal(t, last, head)
	})

	t.Run("repeated Head calls return the same block without consuming it", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b := blockWithRound(1)
		require.NoError(t, c.Append(b))

		for range 3 {
			head, err := c.Head()
			require.NoError(t, err)
			require.Equal(t, b, head)
		}
	})
}

func TestChain_Append(t *testing.T) {
	t.Parallel()

	t.Run("returns ErrNilBlock for a nil block", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		err := c.Append(nil)
		require.ErrorIs(t, err, ErrNilBlock)
	})

	t.Run("does not change Head after a failed nil append", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b := blockWithRound(1)
		require.NoError(t, c.Append(b))

		require.ErrorIs(t, c.Append(nil), ErrNilBlock)

		head, err := c.Head()
		require.NoError(t, err)
		require.Equal(t, b, head)
	})

	t.Run("accepts first block regardless of its round number", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.NoError(t, c.Append(blockWithRound(42)))
	})

	t.Run("returns ErrNonContiguousBlock when round skips ahead", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b1 := blockWithRound(1)
		require.NoError(t, c.Append(b1))

		err := c.Append(blockLinked(3, b1.Header.HeaderHash))
		require.ErrorIs(t, err, ErrNonContiguousBlock)
	})

	t.Run("returns ErrNonContiguousBlock when round goes backwards", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.NoError(t, c.Append(blockWithRound(2)))

		err := c.Append(blockWithRound(1))
		require.ErrorIs(t, err, ErrNonContiguousBlock)
	})

	t.Run("returns ErrNonContiguousBlock when round is the same as head", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b1 := blockWithRound(1)
		require.NoError(t, c.Append(b1))

		err := c.Append(blockLinked(1, b1.Header.HeaderHash))
		require.ErrorIs(t, err, ErrNonContiguousBlock)
	})

	t.Run("returns ErrPreviousHashMismatch when PreviousHash does not match head HeaderHash", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.NoError(t, c.Append(blockWithRound(1)))

		wrongHash := []byte{0xFF}
		err := c.Append(blockLinked(2, wrongHash))
		require.ErrorIs(t, err, ErrPreviousHashMismatch)
	})

	t.Run("returns ErrPreviousHashMismatch when PreviousHash is nil", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.NoError(t, c.Append(blockWithRound(1)))

		err := c.Append(blockWithRound(2)) // PreviousHash is nil
		require.ErrorIs(t, err, ErrPreviousHashMismatch)
	})

	t.Run("does not change Head after a failed contiguity check", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b := blockWithRound(1)
		require.NoError(t, c.Append(b))

		require.ErrorIs(t, c.Append(blockLinked(5, b.Header.HeaderHash)), ErrNonContiguousBlock)

		head, err := c.Head()
		require.NoError(t, err)
		require.Equal(t, b, head)
	})

	t.Run("does not change Head after a failed hash check", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		b := blockWithRound(1)
		require.NoError(t, c.Append(b))

		require.ErrorIs(t, c.Append(blockLinked(2, []byte{0xFF})), ErrPreviousHashMismatch)

		head, err := c.Head()
		require.NoError(t, err)
		require.Equal(t, b, head)
	})
}

func TestChain_Len(t *testing.T) {
	t.Parallel()

	t.Run("is zero on an empty chain", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.Equal(t, uint64(0), c.Len())
	})

	t.Run("increments by one for each successful append", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		var prevHash []byte
		for i := uint64(1); i <= 5; i++ {
			b := blockLinked(i, prevHash)
			require.NoError(t, c.Append(b))
			require.Equal(t, i, c.Len())
			prevHash = b.Header.HeaderHash
		}
	})

	t.Run("does not increment after a failed nil append", func(t *testing.T) {
		t.Parallel()

		c := NewChain()
		require.NoError(t, c.Append(blockWithRound(1)))
		require.ErrorIs(t, c.Append(nil), ErrNilBlock)
		require.Equal(t, uint64(1), c.Len())
	})
}

// blockWithRound creates a chain block whose HeaderHash encodes the round number.
// PreviousHash is not set; use blockLinked to build a properly linked sequence.
func blockWithRound(round uint64) *data.BlockOnChain {
	return &data.BlockOnChain{
		Header: data.ChainBlockHeader{
			Round:      round,
			HeaderHash: []byte{byte(round)},
		},
	}
}

// blockLinked creates a chain block that is properly linked to prevHash.
func blockLinked(round uint64, prevHash []byte) *data.BlockOnChain {
	return &data.BlockOnChain{
		Header: data.ChainBlockHeader{
			Round:        round,
			HeaderHash:   []byte{byte(round)},
			PreviousHash: prevHash,
		},
	}
}
