package labeling

import (
	"testing"

	"github.com/stretchr/testify/require"

	agent2 "moa-chain/agent"
)

func TestAgent_Label(t *testing.T) {
	a := &agent{}
	_, err := a.Label(nil)
	require.Equal(t, err, agent2.ErrNotImplemented)
}

func TestAgent_Answer(t *testing.T) {
	a := &agent{}
	_, err := a.Answer(nil)
	require.Equal(t, err, agent2.ErrNotImplemented)
}
