package state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestRoundState_ExecutedPromptsMessages(t *testing.T) {
	t.Parallel()

	t.Run("should add and get executed prompts messages", func(t *testing.T) {
		t.Parallel()

		roundState := NewRoundState()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		message := &data.AnswersBlockMessage{
			SenderID:  "validator-1",
			BlockHash: []byte("execution-result-hash"),
		}

		err := roundState.AddExecutedPromptsMessage(roundKey, message)
		require.NoError(t, err)

		messages, err := roundState.GetExecutedPromptsMessages(roundKey)
		require.NoError(t, err)
		require.Equal(t, []*data.AnswersBlockMessage{message}, messages)
	})

	t.Run("should return ErrNilExecutedPromptsMessage when message is nil", func(t *testing.T) {
		t.Parallel()

		roundState := NewRoundState()

		err := roundState.AddExecutedPromptsMessage(data.RoundKey{}, nil)

		require.Equal(t, ErrNilExecutedPromptsMessage, err)
	})

	t.Run("should return ErrExecutedPromptsAlreadyExistsForSender when sender already submitted", func(t *testing.T) {
		t.Parallel()

		roundState := NewRoundState()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		message := &data.AnswersBlockMessage{SenderID: "validator-1"}

		err := roundState.AddExecutedPromptsMessage(roundKey, message)
		require.NoError(t, err)

		err = roundState.AddExecutedPromptsMessage(roundKey, message)

		require.Equal(t, ErrExecutedPromptsAlreadyExistsForSender, err)
	})

	t.Run("should return ErrNoExecutedPromptsForCurrentRoundKey when no messages exist", func(t *testing.T) {
		t.Parallel()

		roundState := NewRoundState()

		messages, err := roundState.GetExecutedPromptsMessages(data.RoundKey{})

		require.Nil(t, messages)
		require.Equal(t, ErrNoExecutedPromptsForCurrentRoundKey, err)
	})

	t.Run("should clear executed prompts messages", func(t *testing.T) {
		t.Parallel()

		roundState := NewRoundState()
		roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
		err := roundState.AddExecutedPromptsMessage(roundKey, &data.AnswersBlockMessage{SenderID: "validator-1"})
		require.NoError(t, err)

		roundState.ClearRoundState(roundKey)
		messages, err := roundState.GetExecutedPromptsMessages(roundKey)

		require.Nil(t, messages)
		require.Equal(t, ErrNoExecutedPromptsForCurrentRoundKey, err)
	})
}

func TestRoundState_ProposedSynthesis(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}

	t.Run("should store and retrieve proposed synthesis", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		msg := &data.ProposedSynthesisMessage{SenderID: "leader"}

		err := rs.SetProposedSynthesis(roundKey, msg)
		require.NoError(t, err)

		retrieved, err := rs.GetProposedSynthesis(roundKey)
		require.NoError(t, err)
		require.Same(t, msg, retrieved)
	})

	t.Run("should return ErrNilProposedSynthesis for nil message", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()

		err := rs.SetProposedSynthesis(roundKey, nil)

		require.Equal(t, ErrNilProposedSynthesis, err)
	})

	t.Run("should return ErrProposedSynthesisAlreadyExists on duplicate", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		msg := &data.ProposedSynthesisMessage{SenderID: "leader"}

		require.NoError(t, rs.SetProposedSynthesis(roundKey, msg))
		err := rs.SetProposedSynthesis(roundKey, msg)

		require.Equal(t, ErrProposedSynthesisAlreadyExists, err)
	})

	t.Run("should return ErrNoProposedSynthesisForRoundKey when none stored", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()

		retrieved, err := rs.GetProposedSynthesis(roundKey)

		require.Nil(t, retrieved)
		require.Equal(t, ErrNoProposedSynthesisForRoundKey, err)
	})

	t.Run("should clear proposed synthesis on ClearRoundState", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		require.NoError(t, rs.SetProposedSynthesis(roundKey, &data.ProposedSynthesisMessage{SenderID: "leader"}))

		rs.ClearRoundState(roundKey)
		_, err := rs.GetProposedSynthesis(roundKey)

		require.Equal(t, ErrNoProposedSynthesisForRoundKey, err)
	})
}

func TestRoundState_SynthesisVotes(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}

	t.Run("should store and retrieve synthesis votes in voter-ID order", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		voteB := &data.SynthesisVote{VoterID: "validator-b"}
		voteA := &data.SynthesisVote{VoterID: "validator-a"}

		require.NoError(t, rs.AddSynthesisVote(roundKey, voteB))
		require.NoError(t, rs.AddSynthesisVote(roundKey, voteA))

		votes, err := rs.GetSynthesisVotes(roundKey)
		require.NoError(t, err)
		require.Equal(t, []*data.SynthesisVote{voteA, voteB}, votes)
	})

	t.Run("should return ErrNilSynthesisVote for nil vote", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()

		err := rs.AddSynthesisVote(roundKey, nil)

		require.Equal(t, ErrNilSynthesisVote, err)
	})

	t.Run("should return ErrSynthesisVoteAlreadyExistsForVoter on duplicate voter", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		vote := &data.SynthesisVote{VoterID: "validator-a"}

		require.NoError(t, rs.AddSynthesisVote(roundKey, vote))
		err := rs.AddSynthesisVote(roundKey, vote)

		require.Equal(t, ErrSynthesisVoteAlreadyExistsForVoter, err)
	})

	t.Run("should return ErrNoSynthesisVotesForRoundKey when none stored", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()

		votes, err := rs.GetSynthesisVotes(roundKey)

		require.Nil(t, votes)
		require.Equal(t, ErrNoSynthesisVotesForRoundKey, err)
	})

	t.Run("should clear synthesis votes on ClearRoundState", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		require.NoError(t, rs.AddSynthesisVote(roundKey, &data.SynthesisVote{VoterID: "validator-a"}))

		rs.ClearRoundState(roundKey)
		_, err := rs.GetSynthesisVotes(roundKey)

		require.Equal(t, ErrNoSynthesisVotesForRoundKey, err)
	})
}

func TestRoundState_SynthesisCertificate(t *testing.T) {
	t.Parallel()

	roundKey := data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}

	t.Run("should store certificate and report it as set", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		cert := &data.AggregatedSynthesisVotes{SenderID: "leader"}

		require.False(t, rs.IsSynthesisCertificateSet(roundKey))
		err := rs.SetSynthesisCertificate(roundKey, cert)
		require.NoError(t, err)
		require.True(t, rs.IsSynthesisCertificateSet(roundKey))
	})

	t.Run("should return ErrNilSynthesisCertificate for nil certificate", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()

		err := rs.SetSynthesisCertificate(roundKey, nil)

		require.Equal(t, ErrNilSynthesisCertificate, err)
	})

	t.Run("should return ErrSynthesisCertificateAlreadyExists on duplicate", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		cert := &data.AggregatedSynthesisVotes{SenderID: "leader"}

		require.NoError(t, rs.SetSynthesisCertificate(roundKey, cert))
		err := rs.SetSynthesisCertificate(roundKey, cert)

		require.Equal(t, ErrSynthesisCertificateAlreadyExists, err)
	})

	t.Run("should clear certificate on ClearRoundState", func(t *testing.T) {
		t.Parallel()

		rs := NewRoundState()
		require.NoError(t, rs.SetSynthesisCertificate(roundKey, &data.AggregatedSynthesisVotes{SenderID: "leader"}))

		rs.ClearRoundState(roundKey)

		require.False(t, rs.IsSynthesisCertificateSet(roundKey))
	})
}
