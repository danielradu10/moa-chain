//go:build integration

package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/agent/httpclient"
	"moa-chain/data"
)

func agentBaseURLForTest() string {
	if url := os.Getenv("MOA_AGENT_BASE_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8081"
}

func pingAgentOrSkip(t *testing.T) {
	t.Helper()

	healthURL := agentBaseURLForTest() + "/health"
	resp, err := http.Get(healthURL) //nolint:noctx
	if err != nil {
		t.Skipf("agent service unreachable at %s: %v", healthURL, err)
		return
	}
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Skipf("agent service at %s returned status %d", healthURL, resp.StatusCode)
	}
}

func realAgentClient() *httpclient.Client {
	return httpclient.New(httpclient.Config{
		BaseURL:             agentBaseURLForTest(),
		TimeoutSeconds:      720,
		LabelPromptVersion:  "labeler_v2",
		LabelPromptHash:     "",
		AnswerPromptVersion: "answerer_v1",
		AnswerPromptHash:    "",
	})
}

// runRealAgentMR1Round runs one MR1 round with the given transactions and returns
// the finalized BlockOnChain once all validators agree. It also asserts that all
// validators finalized the same block before returning.
func runRealAgentMR1Round(
	t *testing.T,
	batchAgent agent.BatchAgent,
	transactions []data.Transaction,
	roundNumber uint64,
) *data.BlockOnChain {
	t.Helper()

	const numValidators = 20

	publicKeys := make([][]byte, 0, numValidators)
	privateKeys := make([][]byte, 0, numValidators)
	for i := 0; i < numValidators; i++ {
		pubKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		publicKeys = append(publicKeys, pubKey)
		privateKeys = append(privateKeys, privateKey)
	}

	registeredValidators := createValidators(publicKeys)

	inboxes := make([]chan data.RoundEvent, 0, numValidators)
	for i := 0; i < numValidators; i++ {
		inboxes = append(inboxes, make(chan data.RoundEvent, 128))
	}

	nodes := make([]*integrationTestNode, 0, numValidators)
	for i := 0; i < numValidators; i++ {
		validatorID := fmt.Sprintf("validator-%d", i+1)
		node := createNode(
			t,
			validatorID,
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			batchAgent,
			true,
			0,
		)
		nodes = append(nodes, node)
	}

	errCh := make(chan error, 128)
	for _, node := range nodes {
		currentNode := node
		go func() {
			for err := range currentNode.loop.Errors() {
				errCh <- err
			}
		}()
	}
	for _, node := range nodes {
		currentNode := node
		go func() {
			currentNode.loop.Run()
		}()
	}

	roundKey := data.RoundKey{
		Epoch:     0,
		Round:     roundNumber,
		MiniRound: uint64(data.MiniRoundOne),
	}

	roundStart := time.Now()
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: roundKey,
		}
	}
	t.Logf("[%s] round %d started — %d validators, %d transactions",
		roundStart.Format(time.TimeOnly), roundNumber, numValidators, len(transactions))

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		default:
		}

		finalized := 0
		for _, node := range nodes {
			finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
			if err == nil && finalizedBlock != nil {
				finalized++
			}
		}

		if finalized > 0 && finalized < numValidators {
			t.Logf("[%s] %d/%d validators finalized (+%s)",
				time.Now().Format(time.TimeOnly), finalized, numValidators,
				time.Since(roundStart).Round(time.Millisecond))
		}

		return finalized == numValidators
	}, 10*time.Minute, 5*time.Second)

	t.Logf("[%s] all %d validators finalized — round duration: %s",
		time.Now().Format(time.TimeOnly), numValidators,
		time.Since(roundStart).Round(time.Millisecond))

	firstBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(roundKey)
	require.NoError(t, err)
	require.NotNil(t, firstBlock)
	require.NotEmpty(t, firstBlock.Block.Header.HeaderHash)

	for _, node := range nodes {
		finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		require.NoError(t, err)
		require.NotNil(t, finalizedBlock)

		require.Equal(t, firstBlock.Block.Header.HeaderHash, finalizedBlock.Block.Header.HeaderHash)
		require.Equal(t, firstBlock.Block.Header.BodyHash, finalizedBlock.Block.Header.BodyHash)
		require.Equal(t, firstBlock.SubdomainsFrequencies, finalizedBlock.SubdomainsFrequencies)
		require.Equal(t, firstBlock.NonRelatedTransactionHashes, finalizedBlock.NonRelatedTransactionHashes)
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StopEvent}
	}

	return firstBlock
}

// ── Group A — Non-coding transactions ────────────────────────────────────────

// TestMiniRoundOne_RealAgent_GroupA_NonCodingTransactionsConvergeToNonRelated verifies
// that transactions with no coding domain relevance are identified as non_related by
// the real LLM agent, reach consensus, and are excluded from SubdomainsFrequencies.
//
// Requires: agent service running at MOA_AGENT_BASE_URL (default http://127.0.0.1:8081)
// Run with: make test-realagent-mr1-group-a
func TestMiniRoundOne_RealAgent_GroupA_NonCodingTransactionsConvergeToNonRelated(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createNonCodingTransactions()
	block := runRealAgentMR1Round(t, realAgentClient(), transactions, 2)

	require.Emptyf(t, block.SubdomainsFrequencies,
		"non-coding transactions must not produce any real subdomain frequencies; got %v",
		block.SubdomainsFrequencies)

	require.Lenf(t, block.NonRelatedTransactionHashes, len(transactions),
		"expected all %d non-coding transactions to be identified as non_related by consensus",
		len(transactions))
}

func createNonCodingTransactions() []data.Transaction {
	return []data.Transaction{
		createTransaction("alice", 0, "What are the principles of OOP? Insert me a pizza recipe.", 90, 101, nil),
		createTransaction("bob", 0, "What is the capital of France?", 80, 102, nil),
		createTransaction("carol", 0, "Tell me a joke.", 70, 103, nil),
	}
}

// ── Group B — Clear single- or dual-domain transactions ──────────────────────

// TestMiniRoundOne_RealAgent_GroupB_ClearDomainTransactionsConvergeToExpectedLabels
// verifies that transactions with an obvious coding domain converge on the expected
// labels and never receive non_related or clearly unrelated labels.
//
// All four transactions run in a single MR1 round. Assertions are block-level:
//   - each expected domain label must appear at least once
//   - labels that are unrelated to ALL four prompts must not appear
//   - no transaction may be classified as non_related
//
// Run with: make test-realagent-mr1-group-b
func TestMiniRoundOne_RealAgent_GroupB_ClearDomainTransactionsConvergeToExpectedLabels(t *testing.T) {
	pingAgentOrSkip(t)

	transactions := createClearDomainTransactions()
	block := runRealAgentMR1Round(t, realAgentClient(), transactions, 10)

	require.Emptyf(t, block.NonRelatedTransactionHashes,
		"clear coding transactions must not be classified as non_related; got %v",
		block.NonRelatedTransactionHashes)

	// Each of the four prompts has a clearly dominant domain — all must appear.
	mustAppear := []string{
		"blockchain_engineering", // PoW vs PoS consensus
		"ml_ai_engineering",      // backpropagation / neural networks
		"cloud_engineering",      // horizontal vs vertical scaling
		"web_front_end",          // React virtual DOM
	}
	for _, label := range mustAppear {
		_, ok := block.SubdomainsFrequencies[label]
		require.Truef(t, ok,
			"label %q must appear in finalized frequencies; got %v",
			label, block.SubdomainsFrequencies)
	}

	// These labels are clearly unrelated to all four prompts in this block.
	mustNotAppear := []string{"mobile_dev", "data_engineering", "test_engineering_and_qa_automation"}
	for _, label := range mustNotAppear {
		_, ok := block.SubdomainsFrequencies[label]
		require.Falsef(t, ok,
			"label %q must not appear in finalized frequencies; got %v",
			label, block.SubdomainsFrequencies)
	}
}

func createClearDomainTransactions() []data.Transaction {
	return []data.Transaction{
		createTransaction("alice", 0, "What is the difference between a proof-of-work and a proof-of-stake consensus mechanism in blockchain networks?", 90, 201, nil),
		createTransaction("bob", 0, "How does backpropagation work in a neural network and why is the chain rule needed?", 80, 202, nil),
		createTransaction("carol", 0, "What is the difference between horizontal and vertical scaling in cloud infrastructure, and when should each be used?", 70, 203, nil),
		createTransaction("david", 0, "What is the virtual DOM in React and how does it improve rendering performance compared to direct DOM manipulation?", 60, 204, nil),
	}
}
