package hashing

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestComputeSynthesisBlockHash(t *testing.T) {
	t.Parallel()

	t.Run("same payload produces same hash", func(t *testing.T) {
		t.Parallel()

		first := synthesisProposal()
		second := synthesisProposal()

		firstHash, err := ComputeSynthesisBlockHash(first)
		require.NoError(t, err)
		secondHash, err := ComputeSynthesisBlockHash(second)
		require.NoError(t, err)

		require.Equal(t, firstHash, secondHash)
		require.Len(t, firstHash, 32)
	})

	t.Run("SynthesisBlockHash and Signature are excluded from hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		withDifferentMeta := synthesisProposal()
		withDifferentMeta.SynthesisBlockHash = []byte("some-previous-hash")
		withDifferentMeta.Signature = []byte("some-signature")

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		metaHash, err := ComputeSynthesisBlockHash(withDifferentMeta)
		require.NoError(t, err)

		require.Equal(t, baseHash, metaHash)
	})

	t.Run("changing SenderID changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		changed := synthesisProposal()
		changed.SenderID = "different-leader"

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisBlockHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("changing CanonicalBlockHash changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		changed := synthesisProposal()
		changed.CanonicalBlockHash = []byte("different-block")

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisBlockHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("changing AnswerEvidenceHash changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		changed := synthesisProposal()
		changed.AnswerEvidenceHash = []byte("different-evidence")

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisBlockHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("changing a synthesized answer text changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		changed := synthesisProposal()
		changed.SynthesizedAnswers[0].Answer = "a completely different answer"

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisBlockHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("answer order affects hash — caller must pre-sort", func(t *testing.T) {
		t.Parallel()

		base := synthesisProposal()
		reversed := synthesisProposal()
		reversed.SynthesizedAnswers[0], reversed.SynthesizedAnswers[1] = reversed.SynthesizedAnswers[1], reversed.SynthesizedAnswers[0]

		baseHash, err := ComputeSynthesisBlockHash(base)
		require.NoError(t, err)
		reversedHash, err := ComputeSynthesisBlockHash(reversed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, reversedHash)
	})

	t.Run("nil message returns error", func(t *testing.T) {
		t.Parallel()

		_, err := ComputeSynthesisBlockHash(nil)

		require.ErrorIs(t, err, ErrNilBlock)
	})
}

func TestComputeSynthesisVoteHash(t *testing.T) {
	t.Parallel()

	t.Run("same vote produces same hash", func(t *testing.T) {
		t.Parallel()

		first := synthesisVote()
		second := synthesisVote()

		firstHash, err := ComputeSynthesisVoteHash(first)
		require.NoError(t, err)
		secondHash, err := ComputeSynthesisVoteHash(second)
		require.NoError(t, err)

		require.Equal(t, firstHash, secondHash)
		require.Len(t, firstHash, 32)
	})

	t.Run("VoteHash and Signature are excluded from hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisVote()
		withMeta := synthesisVote()
		withMeta.VoteHash = []byte("some-vote-hash")
		withMeta.Signature = []byte("some-signature")

		baseHash, err := ComputeSynthesisVoteHash(base)
		require.NoError(t, err)
		metaHash, err := ComputeSynthesisVoteHash(withMeta)
		require.NoError(t, err)

		require.Equal(t, baseHash, metaHash)
	})

	t.Run("changing VoterID changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisVote()
		changed := synthesisVote()
		changed.VoterID = "different-validator"

		baseHash, err := ComputeSynthesisVoteHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisVoteHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("changing SynthesisBlockHash changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisVote()
		changed := synthesisVote()
		changed.SynthesisBlockHash = []byte("different-synthesis-hash")

		baseHash, err := ComputeSynthesisVoteHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisVoteHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("changing Epoch changes hash", func(t *testing.T) {
		t.Parallel()

		base := synthesisVote()
		changed := synthesisVote()
		changed.Epoch = 99

		baseHash, err := ComputeSynthesisVoteHash(base)
		require.NoError(t, err)
		changedHash, err := ComputeSynthesisVoteHash(changed)
		require.NoError(t, err)

		require.NotEqual(t, baseHash, changedHash)
	})

	t.Run("nil vote returns error", func(t *testing.T) {
		t.Parallel()

		_, err := ComputeSynthesisVoteHash(nil)

		require.ErrorIs(t, err, ErrNilBlock)
	})
}

func synthesisProposal() *data.ProposedSynthesisMessage {
	return &data.ProposedSynthesisMessage{
		Epoch:              1,
		Round:              2,
		MiniRound:          uint64(data.MiniRoundThree),
		SenderID:           "leader",
		CanonicalBlockHash: []byte("canonical-block-hash"),
		AnswerEvidenceHash: []byte("answer-evidence-hash"),
		SynthesizedAnswers: []data.SynthesizedAnswer{
			{TxHash: []byte("tx-1"), Answer: "synthesized answer for tx-1"},
			{TxHash: []byte("tx-2"), Answer: "synthesized answer for tx-2"},
		},
	}
}

func synthesisVote() *data.SynthesisVote {
	return &data.SynthesisVote{
		Epoch:              1,
		Round:              2,
		MiniRound:          uint64(data.MiniRoundThree),
		VoterID:            "validator-a",
		SynthesisBlockHash: []byte("synthesis-block-hash"),
	}
}
