package broadcast_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/testscommon"
)

func newTx(hash string) data.Transaction {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(hash))
	return tx
}

func newInbox() chan data.Transaction {
	return make(chan data.Transaction, 8)
}

func TestTxPeerRegistry_RegisterAndGetAll(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()

	require.NoError(t, reg.Register("node-1", newInbox()))

	peers := reg.GetAll()
	require.Len(t, peers, 1)
	require.Contains(t, peers, "node-1")
}

func TestTxPeerRegistry_DuplicateRegistrationReturnsError(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()

	require.NoError(t, reg.Register("node-1", newInbox()))
	require.ErrorIs(t, reg.Register("node-1", newInbox()), broadcast.ErrValidatorAlreadyExists)
}

func TestTxPeerRegistry_GetAllReturnsCopy(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()
	require.NoError(t, reg.Register("node-1", newInbox()))

	first := reg.GetAll()
	require.NoError(t, reg.Register("node-2", newInbox()))
	second := reg.GetAll()

	require.Len(t, first, 1)
	require.Len(t, second, 2)
}

func TestTxBroadcaster_DeliversToAllPeersExceptSender(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()

	inboxes := map[string]chan data.Transaction{
		"node-1": newInbox(),
		"node-2": newInbox(),
		"node-3": newInbox(),
	}
	for id, ch := range inboxes {
		require.NoError(t, reg.Register(id, ch))
	}

	b := broadcast.NewTxBroadcaster(reg)
	b.BroadcastTransaction(newTx("tx-1"), "node-1")

	require.Empty(t, inboxes["node-1"], "sender must not receive its own broadcast")
	require.Len(t, inboxes["node-2"], 1)
	require.Len(t, inboxes["node-3"], 1)
}

func TestTxBroadcaster_EmptyRegistryIsNoOp(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()
	b := broadcast.NewTxBroadcaster(reg)

	require.NotPanics(t, func() {
		b.BroadcastTransaction(newTx("tx-1"), "node-1")
	})
}

func TestTxBroadcaster_UnknownSenderBroadcastsToAll(t *testing.T) {
	reg := broadcast.NewTxPeerRegistry()

	inboxes := map[string]chan data.Transaction{
		"node-1": newInbox(),
		"node-2": newInbox(),
	}
	for id, ch := range inboxes {
		require.NoError(t, reg.Register(id, ch))
	}

	b := broadcast.NewTxBroadcaster(reg)
	b.BroadcastTransaction(newTx("tx-1"), "external-client")

	require.Len(t, inboxes["node-1"], 1)
	require.Len(t, inboxes["node-2"], 1)
}
