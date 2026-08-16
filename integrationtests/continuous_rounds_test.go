package integrationtests

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

// TestContinuousRounds verifies the end-to-end full-round sequence:
// MR1 → MR2 → MR3 → chain promotion → blockchainState update → next round starts.
//
// Round 2 has real transactions; subsequent rounds have an empty mempool and
// complete vacuously. The test waits for at least targetRound blocks to appear
// on chain, then stops nodes and asserts protocol invariants.
//
// After stopping the test asserts on every node:
//   - chain head is at least at targetRound (block promoted by finalizeRound)
//   - chain head has a non-empty HeaderHash
//   - blockchainState records at least targetRound, MiniRoundThree (finalizeRound ran)
func TestContinuousRounds(t *testing.T) {
	t.Parallel()

	const numValidators = 5
	const numRounds = 1 // one full round exercised (round 2; genesis is round 1)

	publicKeys, privateKeys := generateScenarioKeys(t, numValidators)
	registeredValidators := createScenarioValidators(publicKeys)

	transactions := continuousRoundTransactions()

	inboxes := makeScenarioInboxes(numValidators)

	// Full-broadcast: every node delivers to every other node.
	peersRegistry := broadcast.NewPeerRegistry()
	for i, v := range registeredValidators {
		require.NoError(t, peersRegistry.Register(v.PublicID(), inboxes[i]))
	}
	broadcaster := broadcast.NewBroadcaster(peersRegistry)

	batchAgent := continuousRoundAgent()

	nodes := make([]*integrationTestNode, numValidators)
	for i, v := range registeredValidators {
		nodes[i] = createNodeWithBroadcaster(
			t,
			v.PublicID(),
			privateKeys[i],
			registeredValidators,
			inboxes[i],
			cloneTransactions(transactions),
			batchAgent,
			broadcaster,
			false,
			0,
			validators.CommitteeStrategyHalf,
			0,
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

	targetRound := uint64(1 + numRounds) // genesis round 1 + numRounds completed
	waitForChainRound(t, nodes, targetRound, 30*time.Second)

	stopNodes()

	require.NoError(t, firstScenarioError(nodes))

	for _, node := range nodes {
		requireChainReachedAtLeastRound(t, node, targetRound)
		requireBlockchainStateUpdatedAtLeast(t, node, targetRound)
	}
}

// waitForChainRound blocks until every node's chain head is at or beyond
// targetRound, or until timeout elapses.
func waitForChainRound(t *testing.T, nodes []*integrationTestNode, targetRound uint64, timeout time.Duration) {
	t.Helper()
	require.Eventually(t, func() bool {
		for _, node := range nodes {
			head, err := node.chain.Head()
			if err != nil || head.Header.Round < targetRound {
				return false
			}
		}
		return true
	}, timeout, 50*time.Millisecond)
}

// requireChainReachedAtLeastRound asserts the node's chain head is at or beyond
// targetRound. Continuous rounds can advance the chain past targetRound before
// nodes are stopped, so equality cannot be required.
func requireChainReachedAtLeastRound(t *testing.T, node *integrationTestNode, targetRound uint64) {
	t.Helper()
	head, err := node.chain.Head()
	require.NoError(t, err)
	require.GreaterOrEqual(t, head.Header.Round, targetRound,
		"node %s chain head should be at least at round %d", node.id, targetRound)
	require.NotEmpty(t, head.Header.HeaderHash,
		"node %s chain head should have a non-empty HeaderHash", node.id)
}

// requireBlockchainStateUpdatedAtLeast asserts that finalizeRound updated
// blockchainState to at least completedRound with MiniRoundThree.
func requireBlockchainStateUpdatedAtLeast(t *testing.T, node *integrationTestNode, completedRound uint64) {
	t.Helper()
	round, err := node.blockchainState.CurrentRound()
	require.NoError(t, err)
	require.GreaterOrEqual(t, round, completedRound,
		"node %s blockchainState.CurrentRound should be at least %d", node.id, completedRound)

	miniRound, err := node.blockchainState.CurrentMiniRound()
	require.NoError(t, err)
	require.Equal(t, uint64(data.MiniRoundThree), miniRound,
		"node %s blockchainState.CurrentMiniRound should be MiniRoundThree after full round", node.id)
}

// continuousRoundTransactions returns a small set of transactions that will
// be proposed in round 2, giving the network real work to do.
func continuousRoundTransactions() []data.Transaction {
	return []data.Transaction{
		createTransaction(
			"alice", 0,
			"Write a concurrent Go HTTP server with graceful shutdown and middleware support.",
			90, 1,
			[]string{"back_end_with_apis", "systems_programming"},
		),
		createTransaction(
			"bob", 0,
			"Build a React dashboard that fetches live metrics and renders interactive charts.",
			80, 2,
			[]string{"web_front_end", "data_engineering"},
		),
	}
}

// continuousRoundAgent returns a mocked BatchAgent suitable for running the
// full MR1 → MR2 → MR3 pipeline without a live Python service.
func continuousRoundAgent() *testscommon.LabelerStub {
	return &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			results := make([]agent.LabelResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.LabelResult{
					TxHash: tx.GetTxHash(),
					Labels: []string{"back_end_with_apis", "systems_programming"},
				}
			}
			return results, nil
		},

		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			results := make([]agent.AnswerResult, len(txs))
			for i, tx := range txs {
				results[i] = agent.AnswerResult{
					TxHash: tx.GetTxHash(),
					Answer: fmt.Sprintf("answer for %x", tx.GetTxHash()),
				}
			}
			return results, nil
		},

		// JudgeAnswersCalled is intentionally nil: integrationProtocolAgent
		// falls back to returning "correct" for all candidates when the
		// delegate returns ErrNotImplemented.

		SynthesizeBatchCalled: func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
			results := make([]agent.SynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.SynthesisResult{
					TxHash: req.Tx.GetTxHash(),
					Answer: fmt.Sprintf("synthesized answer for %x", req.Tx.GetTxHash()),
				}
			}
			return results, nil
		},

		EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
			results := make([]agent.EvaluateSynthesisResult, len(requests))
			for i, req := range requests {
				results[i] = agent.EvaluateSynthesisResult{
					TxHash:   req.Tx.GetTxHash(),
					Approved: true,
				}
			}
			return results, nil
		},
	}
}
