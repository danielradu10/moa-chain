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
