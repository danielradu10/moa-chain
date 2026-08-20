package txpipeline_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/txpipeline"
)

// mempoolStub records AddTransaction calls for assertion in tests.
type mempoolStub struct {
	mu   sync.Mutex
	txs  []data.Transaction
	err  error
}

func (m *mempoolStub) AddTransaction(tx data.Transaction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.txs = append(m.txs, tx)
	return nil
}

func (m *mempoolStub) SelectTransactions(_ state.AccountsState) []data.Transaction { return nil }
func (m *mempoolStub) RemoveTransactions(_ [][]byte)                               {}

func (m *mempoolStub) added() []data.Transaction {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]data.Transaction, len(m.txs))
	copy(result, m.txs)
	return result
}

func newTestTx(hash string) data.Transaction {
	tx := &testscommon.TransactionStub{}
	tx.SetTxHash([]byte(hash))
	return tx
}

func newPreprocessor(batchAgent agent.BatchAgent, pool *mempoolStub) (txpipeline.TxPreprocessor, txpipeline.PrecomputedStore) {
	store := txpipeline.NewPrecomputedStore()
	p := txpipeline.NewTxPreprocessor(txpipeline.TxPreprocessorArgs{
		Agent:   batchAgent,
		Store:   store,
		Mempool: pool,
	})
	return p, store
}

func TestTxPreprocessor_HappyPath(t *testing.T) {
	pool := &mempoolStub{}
	agentStub := &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			return []agent.LabelResult{{TxHash: txs[0].GetTxHash(), Labels: []string{"math"}}}, nil
		},
		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			return []agent.AnswerResult{{TxHash: txs[0].GetTxHash(), Answer: "42"}}, nil
		},
	}

	p, store := newPreprocessor(agentStub, pool)
	p.Start()
	defer p.Stop()

	tx := newTestTx("tx-1")
	p.Enqueue(tx)

	require.Eventually(t, func() bool {
		return len(pool.added()) == 1
	}, 2*time.Second, 5*time.Millisecond)

	labels, ok := store.GetLabels(tx.GetTxHash())
	require.True(t, ok)
	require.Equal(t, []string{"math"}, labels)

	answer, ok := store.GetAnswer(tx.GetTxHash())
	require.True(t, ok)
	require.Equal(t, "42", answer)
}

func TestTxPreprocessor_LabelFailure_TxNotAddedToMempool(t *testing.T) {
	pool := &mempoolStub{}
	agentStub := &testscommon.LabelerStub{
		LabelBatchCalled: func(_ []data.Transaction) ([]agent.LabelResult, error) {
			return nil, errors.New("label service unavailable")
		},
	}

	p, _ := newPreprocessor(agentStub, pool)
	p.Start()

	p.Enqueue(newTestTx("tx-1"))

	time.Sleep(100 * time.Millisecond)
	p.Stop()

	require.Empty(t, pool.added())
}

func TestTxPreprocessor_AnswerFailure_TxNotAddedToMempool(t *testing.T) {
	pool := &mempoolStub{}
	agentStub := &testscommon.LabelerStub{
		AnswerBatchCalled: func(_ []data.Transaction) ([]agent.AnswerResult, error) {
			return nil, errors.New("answer service unavailable")
		},
	}

	p, _ := newPreprocessor(agentStub, pool)
	p.Start()

	p.Enqueue(newTestTx("tx-1"))

	time.Sleep(100 * time.Millisecond)
	p.Stop()

	require.Empty(t, pool.added())
}

func TestTxPreprocessor_MultipleTransactionsProcessedConcurrently(t *testing.T) {
	const numTxs = 10

	pool := &mempoolStub{}
	p, _ := newPreprocessor(&testscommon.LabelerStub{}, pool)
	p.Start()
	defer p.Stop()

	for i := range numTxs {
		p.Enqueue(newTestTx(string(rune('a' + i))))
	}

	require.Eventually(t, func() bool {
		return len(pool.added()) == numTxs
	}, 2*time.Second, 5*time.Millisecond)
}

func TestTxPreprocessor_StopDrainsQueuedTransactions(t *testing.T) {
	const numTxs = 5
	pool := &mempoolStub{}
	p, _ := newPreprocessor(&testscommon.LabelerStub{}, pool)
	p.Start()

	for i := range numTxs {
		p.Enqueue(newTestTx(string(rune('a' + i))))
	}
	p.Stop() // must drain the queue before returning

	require.Len(t, pool.added(), numTxs)
}

func TestTxPreprocessor_StopWaitsForInFlightWork(t *testing.T) {
	pool := &mempoolStub{}

	ready := make(chan struct{})
	agentStub := &testscommon.LabelerStub{
		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			<-ready
			return []agent.AnswerResult{{TxHash: txs[0].GetTxHash(), Answer: "done"}}, nil
		},
	}

	p, _ := newPreprocessor(agentStub, pool)
	p.Start()
	p.Enqueue(newTestTx("tx-1"))

	time.Sleep(20 * time.Millisecond)
	close(ready)
	p.Stop()

	require.Len(t, pool.added(), 1)
}
