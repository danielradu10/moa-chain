package broadcast

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestBroadcaster_AnswerClassificationMessages(t *testing.T) {
	t.Parallel()

	registry := NewPeerRegistry()
	leaderChannel := make(chan data.RoundEvent, 1)
	validatorChannel := make(chan data.RoundEvent, 1)
	require.NoError(t, registry.Register("leader", leaderChannel))
	require.NoError(t, registry.Register("validator", validatorChannel))
	broadcaster := NewBroadcaster(registry)

	voteMessage := &data.ConsensusMessage{
		ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
		AnswerClassificationVote: &data.AnswerClassificationVote{JudgeID: "validator"},
	}
	err := broadcaster.SendAnswerClassificationVoteToLeader(voteMessage, "leader")
	require.NoError(t, err)
	require.Equal(t, *voteMessage, (<-leaderChannel).Message)

	certificateMessage := &data.ConsensusMessage{
		ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
		AnswerClassificationCertificate: &data.AnswerClassificationCertificate{SenderID: "leader"},
	}
	err = broadcaster.BroadcastAnswerClassificationCertificate(
		certificateMessage,
		"leader",
		[]string{"leader", "validator"},
	)
	require.NoError(t, err)
	require.Equal(t, *certificateMessage, (<-validatorChannel).Message)
	require.Empty(t, leaderChannel)
}
