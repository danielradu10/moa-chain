package txpipeline_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/txpipeline"
)

// preprocessorSpy records enqueued transactions.
type preprocessorSpy struct {
	enqueuedCh chan data.Transaction
}

func newPreprocessorSpy() *preprocessorSpy {
	return &preprocessorSpy{enqueuedCh: make(chan data.Transaction, 16)}
}

func (s *preprocessorSpy) Enqueue(tx data.Transaction) { s.enqueuedCh <- tx }
func (s *preprocessorSpy) Start()                       {}
func (s *preprocessorSpy) Stop()                        {}

func newInterceptor(inbox chan data.Transaction, preprocessor txpipeline.TxPreprocessor, txBroadcaster broadcast.TxBroadcaster) txpipeline.TxInterceptor {
	return txpipeline.NewTxInterceptor(txpipeline.TxInterceptorArgs{
		Inbox:        inbox,
		Preprocessor: preprocessor,
		Broadcaster:  txBroadcaster,
		SelfID:       "node-self",
	})
}

func makeTx(hash string) data.Transaction {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(hash))
	return tx
}

func TestTxInterceptor_Submit_EnqueuesAndBroadcasts(t *testing.T) {
	spy := newPreprocessorSpy()

	peerInbox := make(chan data.Transaction, 4)
	reg := broadcast.NewTxPeerRegistry()
	require.NoError(t, reg.Register("node-peer", peerInbox))
	b := broadcast.NewTxBroadcaster(reg)

	interceptor := newInterceptor(make(chan data.Transaction, 4), spy, b)
	interceptor.Start()
	defer interceptor.Stop()

	tx := makeTx("tx-1")
	require.NoError(t, interceptor.Submit(tx))

	select {
	case got := <-spy.enqueuedCh:
		require.Equal(t, tx.GetTxHash(), got.GetTxHash())
	case <-time.After(time.Second):
		t.Fatal("preprocessor was not enqueued")
	}

	require.Len(t, peerInbox, 1)
}

func TestTxInterceptor_Submit_DuplicateIsIgnored(t *testing.T) {
	spy := newPreprocessorSpy()
	b := broadcast.NewTxBroadcaster(broadcast.NewTxPeerRegistry())

	interceptor := newInterceptor(make(chan data.Transaction, 4), spy, b)
	interceptor.Start()
	defer interceptor.Stop()

	tx := makeTx("tx-1")
	require.NoError(t, interceptor.Submit(tx))
	require.NoError(t, interceptor.Submit(tx))

	time.Sleep(20 * time.Millisecond)
	require.Len(t, spy.enqueuedCh, 1)
}

func TestTxInterceptor_Submit_NilTransactionReturnsError(t *testing.T) {
	interceptor := newInterceptor(
		make(chan data.Transaction, 4),
		newPreprocessorSpy(),
		broadcast.NewTxBroadcaster(broadcast.NewTxPeerRegistry()),
	)
	interceptor.Start()
	defer interceptor.Stop()

	require.ErrorIs(t, interceptor.Submit(nil), txpipeline.ErrNilTransaction)
}

func TestTxInterceptor_PeerInbox_EnqueuesWithoutBroadcast(t *testing.T) {
	spy := newPreprocessorSpy()

	peerInbox := make(chan data.Transaction, 4)
	reg := broadcast.NewTxPeerRegistry()
	require.NoError(t, reg.Register("node-peer", peerInbox))
	b := broadcast.NewTxBroadcaster(reg)

	inbox := make(chan data.Transaction, 4)
	interceptor := newInterceptor(inbox, spy, b)
	interceptor.Start()
	defer interceptor.Stop()

	tx := makeTx("tx-from-peer")
	inbox <- tx

	select {
	case got := <-spy.enqueuedCh:
		require.Equal(t, tx.GetTxHash(), got.GetTxHash())
	case <-time.After(time.Second):
		t.Fatal("preprocessor was not enqueued")
	}

	require.Empty(t, peerInbox, "peer-delivered tx must not be re-broadcast")
}

func TestTxInterceptor_PeerInbox_DuplicateAcrossSubmitAndInbox(t *testing.T) {
	spy := newPreprocessorSpy()
	b := broadcast.NewTxBroadcaster(broadcast.NewTxPeerRegistry())

	inbox := make(chan data.Transaction, 4)
	interceptor := newInterceptor(inbox, spy, b)
	interceptor.Start()
	defer interceptor.Stop()

	tx := makeTx("tx-1")
	require.NoError(t, interceptor.Submit(tx))

	inbox <- tx

	time.Sleep(50 * time.Millisecond)
	require.Len(t, spy.enqueuedCh, 1, "same tx via Submit then inbox must be processed only once")
}
