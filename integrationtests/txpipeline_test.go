package integrationtests

import (
	"bytes"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

// TestTxPipeline_TransactionAppearsAfterPreprocessingCompletes verifies the
// core guarantee of the transaction pipeline: LLM preprocessing (label + answer)
// runs off the consensus critical path, so rounds keep finalizing with empty
// blocks while preprocessing is in progress, and the transaction only appears
// in a block once preprocessing is complete.
//
// The slow agent blocks on a gate channel, simulating real LLM latency.
// Once at least minEmptyRoundsBeforeTx full rounds have finalized (all empty),
// the gate is released and the test asserts that the next finalized block
// contains the transaction.
func TestTxPipeline_TransactionAppearsAfterPreprocessingCompletes(t *testing.T) {
	t.Parallel()

	const numValidators = 5
	const minEmptyRoundsBeforeTx = 3

	// preprocessingGate blocks LabelBatch and AnswerBatch until closed,
	// simulating the latency of a real LLM inference call.
	preprocessingGate := make(chan struct{})
	slowAgent := slowAgentBlockingOn(preprocessingGate)

	publicKeys, privateKeys := generateScenarioKeys(t, numValidators)
	registeredValidators := createScenarioValidators(publicKeys)
	inboxes := makeScenarioInboxes(numValidators)

	peersRegistry := broadcast.NewPeerRegistry()
	for i, v := range registeredValidators {
		require.NoError(t, peersRegistry.Register(v.PublicID(), inboxes[i]))
	}
	broadcaster := broadcast.NewBroadcaster(peersRegistry)

	nodes := make([]*integrationTestNode, numValidators)
	for i, v := range registeredValidators {
		nodes[i] = createNodeWithBroadcaster(
			t,
			v.PublicID(),
			privateKeys[i],
			registeredValidators,
			inboxes[i],
			nil, // no initial transactions — preprocessor stays running during consensus
			slowAgent,
			broadcaster,
			false,
			false,
			0,
			validators.CommitteeStrategyHalf,
			0,
			nil, nil,
		)
	}

	done := startScenarioNodes(nodes)
	stopNodes := stopScenarioNodesOnce(t, nodes, done)
	t.Cleanup(stopNodes)

	startScenarioRound(inboxes, data.RoundKey{
		Epoch:     0,
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	})

	// Submit the same transaction to every node. Cross-node tx propagation is
	// not yet wired in integration tests, so each node must receive the tx
	// directly to independently populate its own store before it can validate
	// a block that contains it.
	tx := createTransaction(
		"alice", 0,
		"Implement a concurrent hash map in Go.",
		50, 1,
		[]string{"databases"},
	)
	for _, node := range nodes {
		require.NoError(t, node.txInterceptor.Submit(tx))
	}

	// Wait for at least minEmptyRoundsBeforeTx full rounds (MR1→MR2→MR3) to
	// complete. The gate is still closed so every proposer's mempool is empty
	// and all finalized blocks must have no transactions.
	waitForChainLen(t, nodes, 1+minEmptyRoundsBeforeTx, 30*time.Second)

	for _, node := range nodes {
		for _, block := range node.chain.Blocks()[1:] { // skip genesis
			require.Empty(t, block.Body.Transactions,
				"node %s: block at round %d should be empty — preprocessing is still in progress",
				node.id, block.Header.Round)
		}
	}

	// Release the gate: all blocked LabelBatch and AnswerBatch calls complete
	// simultaneously, each node finishes preprocessing, and the tx enters every
	// node's mempool and PrecomputedStore.
	close(preprocessingGate)

	// The proposer for the first round after preprocessing finds the tx in the
	// mempool. Every validator can confirm labels and answers from its own store.
	// We wait until the tx appears in a finalized block on every node rather
	// than waiting for a fixed chain length, because one or more empty rounds
	// may still finalize between the gate opening and the mempool being populated.
	waitForAllNodesToContainTx(t, nodes, tx.GetTxHash(), 30*time.Second)

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	for _, node := range nodes {
		requireChainContainsTx(t, node, tx.GetTxHash())
	}
}

// TestTxPipeline_BroadcastPropagatesTransactionToAllNodes verifies that when
// one node submits a transaction via TxInterceptor.Submit, the raw transaction
// is broadcast to every other node's inbox via TxBroadcaster, each node
// independently preprocesses it (label + answer), and the tx appears in a
// finalized block validated by all nodes using their own PrecomputedStore.
//
// The atomic labelCallCount counter is the hard proof: if broadcast works,
// every node's preprocessor calls LabelBatch once, giving a total of
// numValidators. If any node never received the tx (broadcast failed), that
// node's preprocessor never calls LabelBatch, the counter stays below
// numValidators, and the final assertion fails.
func TestTxPipeline_BroadcastPropagatesTransactionToAllNodes(t *testing.T) {
	t.Parallel()

	const numValidators = 5
	const minEmptyRoundsBeforeTx = 3

	preprocessingGate := make(chan struct{})

	// labelCallCount counts how many nodes' preprocessors actually called
	// LabelBatch. All 5 nodes share the same *LabelerStub, so every call
	// increments this counter regardless of which node triggered it.
	var labelCallCount atomic.Int32
	slowAgent := &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			labelCallCount.Add(1)
			<-preprocessingGate
			results := make([]agent.LabelResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.LabelResult{TxHash: tx.GetTxHash(), Labels: []string{"databases"}}
			}
			return results, nil
		},
		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			<-preprocessingGate
			results := make([]agent.AnswerResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.AnswerResult{TxHash: tx.GetTxHash(), Answer: fmt.Sprintf("answer for %x", tx.GetTxHash())}
			}
			return results, nil
		},
		SynthesizeBatchCalled: func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
			results := make([]agent.SynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.SynthesisResult{TxHash: req.Tx.GetTxHash(), Answer: fmt.Sprintf("synthesized %x", req.Tx.GetTxHash())}
			}
			return results, nil
		},
		EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
			results := make([]agent.EvaluateSynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.EvaluateSynthesisResult{TxHash: req.Tx.GetTxHash(), Approved: true}
			}
			return results, nil
		},
	}

	publicKeys, privateKeys := generateScenarioKeys(t, numValidators)
	registeredValidators := createScenarioValidators(publicKeys)
	inboxes := makeScenarioInboxes(numValidators)

	peersRegistry := broadcast.NewPeerRegistry()
	for i, v := range registeredValidators {
		require.NoError(t, peersRegistry.Register(v.PublicID(), inboxes[i]))
	}
	broadcaster := broadcast.NewBroadcaster(peersRegistry)

	// Shared tx peer registry: each node registers its inbox so that
	// BroadcastTransaction delivers the raw tx to every other node.
	txRegistry := broadcast.NewTxPeerRegistry()

	nodes := make([]*integrationTestNode, numValidators)
	for i, v := range registeredValidators {
		nodes[i] = createNodeWithBroadcaster(
			t,
			v.PublicID(),
			privateKeys[i],
			registeredValidators,
			inboxes[i],
			nil,
			slowAgent,
			broadcaster,
			false,
			false,
			0,
			validators.CommitteeStrategyHalf,
			0,
			txRegistry, nil,
		)
	}

	done := startScenarioNodes(nodes)
	stopNodes := stopScenarioNodesOnce(t, nodes, done)
	t.Cleanup(stopNodes)

	startScenarioRound(inboxes, data.RoundKey{
		Epoch:     0,
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	})

	// Only node 0 submits the tx. The TxBroadcaster delivers it to all other
	// nodes' inboxes; each node's TxInterceptor picks it up and enqueues it
	// for independent preprocessing.
	tx := createTransaction(
		"alice", 0,
		"Implement a concurrent hash map in Go.",
		50, 1,
		[]string{"databases"},
	)
	require.NoError(t, nodes[0].txInterceptor.Submit(tx))

	// Wait until all numValidators preprocessors have reached LabelBatch.
	// This confirms that broadcast delivered the tx to every node before the
	// gate opens — if any node missed the tx, its preprocessor never calls
	// LabelBatch and this assertion times out.
	require.Eventually(t, func() bool {
		return labelCallCount.Load() == int32(numValidators)
	}, 10*time.Second, 5*time.Millisecond,
		"expected LabelBatch to be called by all %d nodes' preprocessors before gate opens; "+
			"got %d — broadcast did not reach all nodes", numValidators, labelCallCount.Load())

	// All preprocessors are now blocked inside LabelBatch. Run empty rounds to
	// prove the tx cannot enter a block while preprocessing is in progress.
	waitForChainLen(t, nodes, 1+minEmptyRoundsBeforeTx, 30*time.Second)

	for _, node := range nodes {
		for _, block := range node.chain.Blocks()[1:] {
			require.Empty(t, block.Body.Transactions,
				"node %s: block at round %d should be empty — preprocessing is still in progress",
				node.id, block.Header.Round)
		}
	}

	// Release preprocessing on all nodes simultaneously.
	close(preprocessingGate)

	// Every node now has the tx in its mempool and store. Wait for it to appear
	// in a finalized block across all nodes.
	waitForAllNodesToContainTx(t, nodes, tx.GetTxHash(), 30*time.Second)

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	for _, node := range nodes {
		requireChainContainsTx(t, node, tx.GetTxHash())
	}
}

// slowAgentBlockingOn returns a batch agent whose LabelBatch and AnswerBatch
// block until gate is closed. Synthesis and evaluation calls are instant so
// they do not hold up the MR3 protocol on empty rounds.
func slowAgentBlockingOn(gate chan struct{}) *testscommon.LabelerStub {
	return &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			<-gate
			results := make([]agent.LabelResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.LabelResult{TxHash: tx.GetTxHash(), Labels: []string{"databases"}}
			}
			return results, nil
		},
		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			<-gate
			results := make([]agent.AnswerResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.AnswerResult{TxHash: tx.GetTxHash(), Answer: fmt.Sprintf("answer for %x", tx.GetTxHash())}
			}
			return results, nil
		},
		SynthesizeBatchCalled: func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
			results := make([]agent.SynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.SynthesisResult{TxHash: req.Tx.GetTxHash(), Answer: fmt.Sprintf("synthesized %x", req.Tx.GetTxHash())}
			}
			return results, nil
		},
		EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
			results := make([]agent.EvaluateSynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.EvaluateSynthesisResult{TxHash: req.Tx.GetTxHash(), Approved: true}
			}
			return results, nil
		},
	}
}

// waitForAllNodesToContainTx blocks until every node's chain contains a block
// with txHash, or until timeout. Fails immediately on a protocol error.
func waitForAllNodesToContainTx(t *testing.T, nodes []*integrationTestNode, txHash []byte, timeout time.Duration) {
	t.Helper()
	var protocolErr error
	require.Eventually(t, func() bool {
		if protocolErr = firstScenarioError(nodes); protocolErr != nil {
			return true
		}
		for _, node := range nodes {
			if !chainContainsTx(node, txHash) {
				return false
			}
		}
		return true
	}, timeout, 50*time.Millisecond)
	require.NoError(t, protocolErr)
}

// requireChainContainsTx asserts that at least one finalized block on node
// contains a transaction with the given hash.
func requireChainContainsTx(t *testing.T, node *integrationTestNode, txHash []byte) {
	t.Helper()
	require.True(t, chainContainsTx(node, txHash),
		"node %s: transaction %x not found in any finalized block", node.id, txHash)
}

func chainContainsTx(node *integrationTestNode, txHash []byte) bool {
	for _, block := range node.chain.Blocks() {
		for _, tx := range block.Body.Transactions {
			if bytes.Equal(tx.GetTxHash(), txHash) {
				return true
			}
		}
	}
	return false
}
