package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

func TestMiniRoundOne_NoErrorsDuringRound(t *testing.T) {
	const numValidators = 7

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
			nil,
			&testscommon.LabelerStub{},
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
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: roundKey,
		}
	}

	require.Never(t, func() bool {
		select {
		case err := <-errCh:
			t.Logf("round loop error: %v", err)
			return true

		default:
			return false
		}
	}, time.Second, 10*time.Millisecond)

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type: data.StopEvent,
		}
	}
}

func TestMiniRoundOne_AllNodesFinalizeSameBlock_NoTransactions(t *testing.T) {
	const numValidators = 100

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
			nil,
			&testscommon.LabelerStub{},
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
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: roundKey,
		}
	}

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		default:
		}

		for _, node := range nodes {
			finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
			if err != nil || finalizedBlock == nil {
				return false
			}
		}

		return true
	}, 5*time.Second, 10*time.Millisecond)

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
		require.Equal(t, firstBlock.Block.Header.Nonce, finalizedBlock.Block.Header.Nonce)
		require.Equal(t, firstBlock.Block.Header.Round, finalizedBlock.Block.Header.Round)
		require.Equal(t, firstBlock.Block.Header.MiniRound, finalizedBlock.Block.Header.MiniRound)
		require.Equal(t, firstBlock.SubdomainsFrequencies, finalizedBlock.SubdomainsFrequencies)
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type: data.StopEvent,
		}
	}
}

func TestMiniRoundOne_AllNodesFinalizeSameBlock_WithTransactions(t *testing.T) {
	const numValidators = 10

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

	transactions := createTransactions()

	labelerStub := &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			results := make([]agent.LabelResult, 0, len(txs))
			for _, tx := range txs {
				labels := tx.GetDomainLabels()
				if len(labels) == 0 {
					return nil, errors.New("transaction has no precomputed labels")
				}
				copiedLabels := make([]string, len(labels))
				copy(copiedLabels, labels)
				results = append(results, agent.LabelResult{TxHash: tx.GetTxHash(), Labels: copiedLabels})
			}
			return results, nil
		},
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
			labelerStub,
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
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: roundKey,
		}
	}

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		default:
		}

		for _, node := range nodes {
			finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
			if err != nil || finalizedBlock == nil {
				return false
			}
		}

		return true
	}, time.Second, 10*time.Millisecond)

	firstBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(roundKey)
	require.NoError(t, err)
	require.NotNil(t, firstBlock)
	require.NotEmpty(t, firstBlock.Block.Header.HeaderHash)

	require.Len(t, firstBlock.Block.Body.Transactions, len(transactions))
	require.Equal(t, expectedSubdomains(), firstBlock.SubdomainsFrequencies)

	for _, node := range nodes {
		finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		require.NoError(t, err)
		require.NotNil(t, finalizedBlock)

		require.Equal(t, firstBlock.Block.Header.HeaderHash, finalizedBlock.Block.Header.HeaderHash)
		require.Equal(t, firstBlock.Block.Header.BodyHash, finalizedBlock.Block.Header.BodyHash)
		require.Equal(t, firstBlock.Block.Header.Nonce, finalizedBlock.Block.Header.Nonce)
		require.Equal(t, firstBlock.Block.Header.Round, finalizedBlock.Block.Header.Round)
		require.Equal(t, firstBlock.Block.Header.MiniRound, finalizedBlock.Block.Header.MiniRound)
		require.Equal(t, firstBlock.SubdomainsFrequencies, finalizedBlock.SubdomainsFrequencies)
		require.Len(t, finalizedBlock.Block.Body.Transactions, len(transactions))
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type: data.StopEvent,
		}
	}
}

func TestMiniRoundOne_AllNodesFinalizeSameBlock_WithAgentGeneratedLabels(t *testing.T) {
	const numValidators = 10

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

	transactions := loadMiniRoundOneTransactionsFixture(t)
	agentLabels := loadAgentLabelsFixtures(t, numValidators)

	validateAgentLabelsFixtures(t, transactions, agentLabels)

	nodes := make([]*integrationTestNode, 0, numValidators)
	for i := 0; i < numValidators; i++ {
		validatorID := fmt.Sprintf("validator-%d", i+1)

		labelerStub := createAgentBackedLabeler(agentLabels[i])

		node := createNode(
			t,
			validatorID,
			privateKeys[i],
			registeredValidators,
			inboxes,
			inboxes[i],
			cloneTransactions(transactions),
			labelerStub,
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
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: roundKey,
		}
	}

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		default:
		}

		for _, node := range nodes {
			finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
			if err != nil || finalizedBlock == nil {
				return false
			}
		}

		return true
	}, time.Second, 10*time.Millisecond)

	firstBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(roundKey)
	require.NoError(t, err)
	require.NotNil(t, firstBlock)
	require.NotEmpty(t, firstBlock.Block.Header.HeaderHash)

	require.Len(t, firstBlock.Block.Body.Transactions, len(transactions))
	require.NotEmpty(t, firstBlock.SubdomainsFrequencies)

	appendConsensusFrequenciesResult(t, firstBlock.SubdomainsFrequencies)

	consensusGroup := selectedConsensusGroupForRound(t, registeredValidators, roundKey)
	requireFinalizedFrequenciesFromValidQuorum(
		t,
		firstBlock.SubdomainsFrequencies,
		consensusGroup,
		agentLabels,
		transactions,
	)

	for _, node := range nodes {
		finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		require.NoError(t, err)
		require.NotNil(t, finalizedBlock)

		require.Equal(t, firstBlock.Block.Header.HeaderHash, finalizedBlock.Block.Header.HeaderHash)
		require.Equal(t, firstBlock.Block.Header.BodyHash, finalizedBlock.Block.Header.BodyHash)
		require.Equal(t, firstBlock.Block.Header.Nonce, finalizedBlock.Block.Header.Nonce)
		require.Equal(t, firstBlock.Block.Header.Round, finalizedBlock.Block.Header.Round)
		require.Equal(t, firstBlock.Block.Header.MiniRound, finalizedBlock.Block.Header.MiniRound)
		require.Equal(t, firstBlock.SubdomainsFrequencies, finalizedBlock.SubdomainsFrequencies)
		require.Len(t, finalizedBlock.Block.Body.Transactions, len(transactions))
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type: data.StopEvent,
		}
	}
}

func createTransactions() []data.Transaction {
	return []data.Transaction{
		createTransaction(
			"alice",
			0,
			"Build a REST API in Go with PostgreSQL, JWT authentication and role-based access control.",
			90,
			1,
			[]string{
				"back_end_with_apis",
				"databases",
				"security",
			},
		),
		createTransaction(
			"bob",
			0,
			"Design a React frontend with reusable components, client-side routing and form validation.",
			80,
			2,
			[]string{
				"web_front_end",
				"test_engineering_and_qa_automation",
				"mobile_dev",
			},
		),
		createTransaction(
			"carol",
			0,
			"Create a data pipeline that ingests logs, transforms them and stores analytics-ready tables.",
			70,
			3,
			[]string{
				"data_engineering",
				"databases",
				"cloud_engineering",
			},
		),
		createTransaction(
			"david",
			0,
			"Implement a smart contract wallet with signature verification and secure transaction execution.",
			60,
			4,
			[]string{
				"blockchain_engineering",
				"security",
				"systems_programming",
			},
		),
		createTransaction(
			"eveline",
			0,
			"Build a mobile application with offline sync, push notifications and API integration.",
			50,
			5,
			[]string{
				"mobile_dev",
				"back_end_with_apis",
				"cloud_engineering",
			},
		),
		createTransaction(
			"frank",
			0,
			"Set up CI/CD pipelines, containerized deployments, monitoring and production infrastructure.",
			40,
			6,
			[]string{
				"dev_ops",
				"cloud_engineering",
				"test_engineering_and_qa_automation",
			},
		),
	}
}

func createTransaction(
	sender string,
	nonce uint64,
	prompt string,
	tip uint64,
	timestamp uint64,
	labels []string,
) data.Transaction {
	tx := mempool.NewTransaction()

	tx.SetNonce(nonce)
	tx.SetSender([]byte(sender))
	tx.SetReceiver([]byte("moa-chain"))
	tx.SetTransferredValue(0)

	tx.SetPrompt([]byte(prompt))
	tx.SetTip(tip)
	tx.SetTimestamp(timestamp)

	tx.SetEstimatedFee(1)

	tx.SetThinkingMode("fast")
	tx.SetUserOutputDimension("short")

	tx.SetDomainLabels(copyStringSlice(labels))

	tx.SetTxHash(computeTestTxHash(
		sender,
		nonce,
		prompt,
		tip,
		timestamp,
	))

	return tx
}

func expectedSubdomains() data.SubdomainsFrequency {
	// The consensus group and quorum are selected deterministically from the 10
	// registered validators for round key {Epoch:0, Round:2, MiniRound:0}.
	// The quorum certificate contains 4 validators, each contributing 1 vote per
	// label per transaction. Labels appearing in N transactions contribute N×4
	// to the block-level frequency map.
	return data.SubdomainsFrequency{
		"back_end_with_apis":                 8,  // alice + eveline
		"databases":                          8,  // alice + carol
		"security":                           8,  // alice + david
		"web_front_end":                      4,  // bob
		"test_engineering_and_qa_automation": 8,  // bob + frank
		"mobile_dev":                         8,  // bob + eveline
		"data_engineering":                   4,  // carol
		"cloud_engineering":                  12, // carol + eveline + frank
		"blockchain_engineering":             4,  // david
		"systems_programming":                4,  // david
		"dev_ops":                            4,  // frank
	}
}

type agentLabelsFixture struct {
	Agent               string                    `json:"agent"`
	LabeledTransactions []labeledTransactionEntry `json:"labeledTransactions"`
}

type labeledTransactionEntry struct {
	TxHash string   `json:"txHash"`
	Labels []string `json:"labels"`
}

type agentLabelsByTxHash struct {
	agent          string
	labelsByTxHash map[string][]string
}

func loadMiniRoundOneTransactionsFixture(t *testing.T) []data.Transaction {
	t.Helper()

	path := filepath.Join("testData", "miniround1", "transactions.json")

	rawData, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixtures []miniRoundOneTransactionFixture
	err = json.Unmarshal(rawData, &fixtures)
	require.NoError(t, err)

	transactions := make([]data.Transaction, 0, len(fixtures))
	for _, fixture := range fixtures {
		transactions = append(transactions, createTransactionFromFixture(fixture))
	}

	return transactions
}

func loadAgentLabelsFixtures(t *testing.T, numAgents int) []agentLabelsByTxHash {
	t.Helper()

	agents := make([]agentLabelsByTxHash, 0, numAgents)

	for i := 1; i <= numAgents; i++ {
		path := filepath.Join("testData", "miniround1", "agents", fmt.Sprintf("agent_%d.json", i))
		agents = append(agents, loadAgentLabelsFixture(t, path))
	}

	return agents
}

func loadAgentLabelsFixture(t *testing.T, path string) agentLabelsByTxHash {
	t.Helper()

	rawData, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture agentLabelsFixture
	err = json.Unmarshal(rawData, &fixture)
	require.NoError(t, err)

	labelsByTxHash := make(map[string][]string, len(fixture.LabeledTransactions))
	for _, labeledTx := range fixture.LabeledTransactions {
		labelsByTxHash[labeledTx.TxHash] = copyStringSlice(labeledTx.Labels)
	}

	return agentLabelsByTxHash{
		agent:          fixture.Agent,
		labelsByTxHash: labelsByTxHash,
	}
}

func createAgentBackedLabeler(agentLabels agentLabelsByTxHash) agent.BatchAgent {
	return &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			results := make([]agent.LabelResult, 0, len(txs))
			for _, tx := range txs {
				txHash := string(tx.GetTxHash())

				labels, ok := agentLabels.labelsByTxHash[txHash]
				if !ok {
					return nil, fmt.Errorf("agent %s has no labels for txHash %s", agentLabels.agent, txHash)
				}

				results = append(results, agent.LabelResult{TxHash: tx.GetTxHash(), Labels: copyStringSlice(labels)})
			}
			return results, nil
		},
	}
}

func validateAgentLabelsFixtures(
	t *testing.T,
	transactions []data.Transaction,
	agents []agentLabelsByTxHash,
) {
	t.Helper()

	txHashes := make([]string, 0, len(transactions))
	for _, tx := range transactions {
		txHashes = append(txHashes, string(tx.GetTxHash()))
	}

	for _, agentLabels := range agents {
		require.NotEmpty(t, agentLabels.agent)

		for _, txHash := range txHashes {
			labels, ok := agentLabels.labelsByTxHash[txHash]
			require.Truef(t, ok, "agent %s has no labels for txHash %s", agentLabels.agent, txHash)

			require.Truef(t, len(labels) >= 1 && len(labels) <= 3,
				"agent %s has invalid labels count for txHash %s: got %d, want 1–3", agentLabels.agent, txHash, len(labels))

			seenLabels := make(map[string]struct{}, len(labels))
			for _, label := range labels {
				_, ok = possibleSubDomains[label]
				require.Truef(t, ok, "agent %s has invalid label %q for txHash %s", agentLabels.agent, label, txHash)

				_, duplicated := seenLabels[label]
				require.Falsef(t, duplicated, "agent %s has duplicated label %q for txHash %s", agentLabels.agent, label, txHash)

				seenLabels[label] = struct{}{}
			}
		}
	}
}

func selectedConsensusGroupForRound(
	t *testing.T,
	registeredValidators []*validators.Validator,
	roundKey data.RoundKey,
) []string {
	t.Helper()

	consensusSelector := validators.NewConsensusSelector()
	validatorRegistry := validators.NewValidatorRegistry(consensusSelector)

	for _, validator := range registeredValidators {
		err := validatorRegistry.Register(validator.PublicID(), validator)
		require.NoError(t, err)
	}

	blockchainStateStub := &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: currentIntegrationTestHeader(),
	}

	err := validatorRegistry.GenerateConsensusGroupMiniRoundOne(blockchainStateStub, roundKey)
	require.NoError(t, err)

	consensusGroup, err := validatorRegistry.ConsensusGroup()
	require.NoError(t, err)

	return consensusGroup
}

func requireFinalizedFrequenciesFromValidQuorum(
	t *testing.T,
	actual data.SubdomainsFrequency,
	consensusGroup []string,
	agents []agentLabelsByTxHash,
	transactions []data.Transaction,
) {
	t.Helper()

	require.NotEmpty(t, consensusGroup)

	quorumSize := consensusQuorum(len(consensusGroup))
	require.LessOrEqual(t, quorumSize, len(consensusGroup))

	leaderID := consensusGroup[0]
	remainingValidators := consensusGroup[1:]
	requiredFollowers := quorumSize - 1

	var matchingQuorum []string
	var visit func(start int, selected []string)
	visit = func(start int, selected []string) {
		if matchingQuorum != nil {
			return
		}

		if len(selected) == requiredFollowers {
			quorum := append([]string{leaderID}, selected...)
			expected := aggregateAgentLabelFrequencies(t, quorum, agents, transactions, uint64(quorumSize))
			if subdomainsFrequenciesEqual(actual, expected) {
				matchingQuorum = copyStringSlice(quorum)
			}
			return
		}

		remainingSlots := requiredFollowers - len(selected)
		for i := start; i <= len(remainingValidators)-remainingSlots; i++ {
			visit(i+1, append(selected, remainingValidators[i]))
		}
	}

	visit(0, nil)

	require.NotNilf(
		t,
		matchingQuorum,
		"finalized subdomain frequencies are not explainable by any quorum of the selected consensus group; consensusGroup=%v actual=%v",
		consensusGroup,
		actual,
	)
}

func aggregateAgentLabelFrequencies(
	t *testing.T,
	validatorIDs []string,
	agents []agentLabelsByTxHash,
	transactions []data.Transaction,
	threshold uint64,
) data.SubdomainsFrequency {
	t.Helper()

	frequencies := make(data.SubdomainsFrequency)

	for _, tx := range transactions {
		txHash := string(tx.GetTxHash())
		txLabelsFrequencies := make(map[string]uint64)

		for _, validatorID := range validatorIDs {
			agentIndex := agentIndexForValidatorID(t, validatorID)
			require.Less(t, agentIndex, len(agents))

			agentLabels := agents[agentIndex]
			labels, ok := agentLabels.labelsByTxHash[txHash]
			require.Truef(t, ok, "agent %s has no labels for txHash %s", agentLabels.agent, txHash)

			for _, label := range labels {
				txLabelsFrequencies[label]++
			}
		}

		for label, frequency := range txLabelsFrequencies {
			if frequency >= threshold {
				frequencies[label] += frequency
			}
		}
	}

	return frequencies
}

func agentIndexForValidatorID(t *testing.T, validatorID string) int {
	t.Helper()

	var validatorNumber int
	_, err := fmt.Sscanf(validatorID, "validator-%d", &validatorNumber)
	require.NoError(t, err)
	require.Greater(t, validatorNumber, 0)

	return validatorNumber - 1
}

func subdomainsFrequenciesEqual(left data.SubdomainsFrequency, right data.SubdomainsFrequency) bool {
	if len(left) != len(right) {
		return false
	}

	for subdomain, leftFrequency := range left {
		if right[subdomain] != leftFrequency {
			return false
		}
	}

	return true
}

func appendConsensusFrequenciesResult(t *testing.T, frequencies data.SubdomainsFrequency) {
	t.Helper()

	result := struct {
		TestName    string                   `json:"testName"`
		Timestamp   string                   `json:"timestamp"`
		Frequencies data.SubdomainsFrequency `json:"frequencies"`
	}{
		TestName:    t.Name(),
		Timestamp:   time.Now().UTC().Format(time.RFC3339Nano),
		Frequencies: frequencies,
	}

	encodedResult, err := json.Marshal(result)
	require.NoError(t, err)

	outputPath := filepath.Join("testData", "miniround1", "results", "consensus_frequencies_results.jsonl")
	err = os.MkdirAll(filepath.Dir(outputPath), 0o755)
	require.NoError(t, err)

	outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	defer func() {
		err = outputFile.Close()
		require.NoError(t, err)
	}()

	_, err = outputFile.Write(append(encodedResult, '\n'))
	require.NoError(t, err)
}
