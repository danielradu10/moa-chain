package labeling

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
)

func TestLabeler_LabelBatch(t *testing.T) {
	l := &labeler{}
	_, err := l.LabelBatch(nil)
	require.Equal(t, agent.ErrNotImplemented, err)
}

func TestLabeler_AnswerBatch(t *testing.T) {
	l := &labeler{}
	_, err := l.AnswerBatch(nil)
	require.Equal(t, agent.ErrNotImplemented, err)
}
