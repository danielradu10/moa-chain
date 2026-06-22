package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/proposing"
	"moa-chain/blockprocessing/validation"
	"moa-chain/broadcast"
	"moa-chain/consensus"
	"moa-chain/consensus/miniround1"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

const integrationTestInitialBalance = uint64(10_000)

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
		LabelCalled: func(tx data.Transaction) ([]string, error) {
			labels := tx.GetDomainLabels()
			if len(labels) == 0 {
				return nil, errors.New("transaction has no precomputed labels")
			}

			copiedLabels := make([]string, len(labels))
			copy(copiedLabels, labels)

			return copiedLabels, nil
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

type integrationTestNode struct {
	id             string
	loop           *consensus.RoundLoop
	blockFinalizer *blockFinalizer.FinalizeBlockComponent
	logger         *logging.NodeLogger
}

func createValidators(pubKeys [][]byte) []*validators.Validator {
	vs := make([]*validators.Validator, 0, len(pubKeys))

	for i, pubkey := range pubKeys {
		v := validators.NewValidator(fmt.Sprintf("validator-%d", i+1), pubkey, 100)
		vs = append(vs, v)
	}

	return vs
}

func currentIntegrationTestHeader() *data.BlockHeader {
	return &data.BlockHeader{
		BodyHash:         []byte("body hash 1"),
		HeaderHash:       []byte("header hash 1"),
		PreviousHash:     []byte("previous hash 0"),
		RootHash:         []byte("root hash 1"),
		PreviousRootHash: []byte("previous root hash 0"),
		Nonce:            1,
		Round:            1,
		MiniRound:        uint64(data.MiniRoundThree),
		Epoch:            0,
	}
}

func createNode(
	t *testing.T,
	validatorID string,
	privateKey []byte,
	registeredValidators []*validators.Validator,
	inboxes []chan data.RoundEvent,
	myInbox chan data.RoundEvent,
	transactions []data.Transaction,
	labeler agent.Agent,
) *integrationTestNode {
	finalizer := blockFinalizer.NewFinalizeBlockComponent()
	nodeLogger, err := createIntegrationTestNodeLogger(t, validatorID)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, nodeLogger.Close())
	})

	logger := nodeLogger.Logger()

	txPool := mempool.NewMemPool(logger)

	for _, tx := range transactions {
		err := txPool.AddTransaction(tx)
		require.NoError(t, err)
	}

	peersRegistry := broadcast.NewPeerRegistry()
	consensusSelector := validators.NewConsensusSelector(logger)
	validatorsRegistry := validators.NewValidatorRegistry(consensusSelector, logger)

	for i, validator := range registeredValidators {
		err := validatorsRegistry.Register(validator.PublicID(), validator)
		require.NoError(t, err)

		err = peersRegistry.Register(validator.PublicID(), inboxes[i])
		require.NoError(t, err)
	}

	loop := createRoundLoop(
		validatorID,
		privateKey,
		txPool,
		peersRegistry,
		validatorsRegistry,
		myInbox,
		finalizer,
		labeler,
		logger,
	)

	require.NotNil(t, loop)

	return &integrationTestNode{
		id:             validatorID,
		loop:           loop,
		blockFinalizer: finalizer,
		logger:         nodeLogger,
	}
}

func createIntegrationTestNodeLogger(t *testing.T, validatorID string) (*logging.NodeLogger, error) {
	t.Helper()

	testName := strings.NewReplacer("/", "_", " ", "_").Replace(t.Name())
	logPath := filepath.Join("logs", testName, validatorID+".log")
	fileLogLevel := logging.ParseIntegrationTestLevel(os.Getenv("MOA_TEST_LOG_LEVEL"))
	consoleLogLevel := logging.ParseIntegrationTestConsoleLevel(os.Getenv("MOA_TEST_CONSOLE_LOG_LEVEL"))

	return logging.NewNodeLoggerWithLevels(validatorID, logPath, fileLogLevel, consoleLogLevel)
}

func createRoundLoop(
	nodeID string,
	privateKey []byte,
	txPool mempool.Mempool,
	peerRegistry broadcast.PeerRegistry,
	validatorRegistry validators.ValidatorRegistry,
	inbox chan data.RoundEvent,
	blockFinalizer blockFinalizer.BlockFinalizer,
	labeler agent.Agent,
	logger *slog.Logger,
) *consensus.RoundLoop {
	currentHeader := currentIntegrationTestHeader()

	blockchainStateStub := &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: currentHeader,
		CurrentRoundValue:       currentHeader.Round,
		CurrentMiniRoundValue:   currentHeader.MiniRound,
		CurrentEpochValue:       currentHeader.Epoch,
	}

	base := createBlockBase(txPool, blockchainStateStub, labeler, logger)
	roundState := state.NewRoundState()

	miniRoundOneHandlerArgs := miniround1.MiniRoundOneHandlerArgs{
		MyID:              nodeID,
		BlockCreator:      proposing.NewBlockCreator(base),
		BlockProcessor:    validation.NewBlockProcessor(base),
		LabelsValidator:   validation.NewLabelsValidator(logger),
		RoundState:        roundState,
		Broadcaster:       broadcast.NewBroadcaster(peerRegistry, logger),
		Signer:            signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry: validatorRegistry,
		BlockchainState:   blockchainStateStub,
		BlockFinalizer:    blockFinalizer,
		Logger:            logger,
	}

	miniRoundOneHandler := miniround1.NewMiniRoundOneHandler(miniRoundOneHandlerArgs)

	roundHandlerArgs := consensus.RoundHandlerArgs{
		SelfID:              nodeID,
		CurrentStep:         data.StepIdle,
		CurrentRoundKey:     data.RoundKey{},
		MiniRoundOneHandler: miniRoundOneHandler,
		Logger:              logger,
	}

	roundHandler := consensus.NewRoundHandler(roundHandlerArgs)

	return consensus.NewRoundLoop(roundHandler, inbox, logger)
}

func createBlockBase(
	mempool mempool.Mempool,
	blockchainState state.BlockchainState,
	labelerCalled agent.Agent,
	logger *slog.Logger,
) blockprocessing.Base {
	aliceAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)
	bobAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)
	carolAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)
	davidAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)
	evelineAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)
	frankAccount := testscommon.NewAccountHandlerStub(0, integrationTestInitialBalance)

	escrowAccount := testscommon.NewAccountHandlerStub(0, 0)

	accounts := map[string]*testscommon.AccountHandlerStub{
		"alice":   aliceAccount,
		"bob":     bobAccount,
		"carol":   carolAccount,
		"david":   davidAccount,
		"eveline": evelineAccount,
		"frank":   frankAccount,
	}

	accountSnapshotStub := &testscommon.AccountsSnapshotStub{
		Accounts:      accounts,
		EscrowAccount: escrowAccount,
	}

	accountSnapshotFactoryMock := testscommon.AccountsSnapshotFactoryStub{
		Snapshot: accountSnapshotStub,
	}

	accountStateStub := testscommon.NewAccountStateStub()
	_ = accountStateStub.AddAccount("alice", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("bob", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("carol", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("david", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("eveline", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("frank", 0, integrationTestInitialBalance)
	_ = accountStateStub.AddAccount("escrow", 0, 0)

	return blockprocessing.Base{
		AccountsSnapshotFactory: &accountSnapshotFactoryMock,
		BlockchainState:         blockchainState,
		Agent:                   labelerCalled,
		AccountState:            accountStateStub,
		Mempool:                 mempool,
		Logger:                  logger,
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
				"cloud_engineering",
				"dev_ops",
				"test_engineering_and_qa_automation",
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
				"security",
				"back_end_with_apis",
				"databases",
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
				"ml_ai_engineering",
				"dev_ops",
				"back_end_with_apis",
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
				"databases",
				"cloud_engineering",
				"dev_ops",
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
				"web_front_end",
				"security",
				"databases",
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
				"systems_programming",
				"security",
				"data_engineering",
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

func cloneTransactions(transactions []data.Transaction) []data.Transaction {
	clonedTransactions := make([]data.Transaction, 0, len(transactions))

	for _, tx := range transactions {
		clonedTx := mempool.NewTransaction()

		clonedTx.SetNonce(tx.GetNonce())
		clonedTx.SetSender(copyBytes(tx.GetSender()))
		clonedTx.SetReceiver(copyBytes(tx.GetReceiver()))
		clonedTx.SetTransferredValue(tx.GetTransferredValue())

		clonedTx.SetPrompt(copyBytes(tx.GetPrompt()))
		clonedTx.SetTip(tx.GetTip())
		clonedTx.SetTimestamp(tx.GetTimestamp())

		clonedTx.SetTxHash(copyBytes(tx.GetTxHash()))

		clonedTx.SetDomainLabels(copyStringSlice(tx.GetDomainLabels()))

		clonedTx.SetNumInputTokens(tx.GetNumInputTokens())
		clonedTx.SetUserOutputDimension(tx.GetUserOutputDimension())
		clonedTx.SetThinkingMode(tx.GetThinkingMode())

		clonedTx.SetEstimatedConsumption(tx.GetEstimatedConsumption())
		clonedTx.SetEstimatedFee(tx.GetEstimatedFee())
		clonedTx.SetEstimatedScore(tx.GetEstimatedScore())

		clonedTransactions = append(clonedTransactions, clonedTx)
	}

	return clonedTransactions
}

func expectedSubdomains() data.SubdomainsFrequency {
	return data.SubdomainsFrequency{
		"back_end_with_apis":                 16,
		"databases":                          20,
		"security":                           20,
		"web_front_end":                      8,
		"test_engineering_and_qa_automation": 12,
		"data_engineering":                   8,
		"cloud_engineering":                  20,
		"blockchain_engineering":             4,
		"systems_programming":                8,
		"mobile_dev":                         8,
		"dev_ops":                            16,
		"ml_ai_engineering":                  4,
	}
}

func computeTestTxHash(
	sender string,
	nonce uint64,
	prompt string,
	tip uint64,
	timestamp uint64,
) []byte {
	hasher := sha256.New()

	hasher.Write([]byte("integration-test-transaction-v1"))
	writeTestString(hasher, sender)
	writeTestUint64(hasher, nonce)
	writeTestString(hasher, prompt)
	writeTestUint64(hasher, tip)
	writeTestUint64(hasher, timestamp)

	return hasher.Sum(nil)
}

func copyBytes(input []byte) []byte {
	if input == nil {
		return nil
	}

	output := make([]byte, len(input))
	copy(output, input)

	return output
}

func copyStringSlice(input []string) []string {
	if input == nil {
		return nil
	}

	output := make([]string, len(input))
	copy(output, input)

	return output
}

func writeTestUint64(
	hasher interface{ Write([]byte) (int, error) },
	value uint64,
) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], value)
	_, _ = hasher.Write(buffer[:])
}

func writeTestString(
	hasher interface{ Write([]byte) (int, error) },
	value string,
) {
	writeTestUint64(hasher, uint64(len(value)))
	_, _ = hasher.Write([]byte(value))
}

var possibleSubDomains = map[string]struct{}{
	"systems_programming":                {},
	"web_front_end":                      {},
	"back_end_with_apis":                 {},
	"ml_ai_engineering":                  {},
	"data_engineering":                   {},
	"dev_ops":                            {},
	"security":                           {},
	"mobile_dev":                         {},
	"test_engineering_and_qa_automation": {},
	"blockchain_engineering":             {},
	"cloud_engineering":                  {},
	"databases":                          {},
}

type miniRoundOneTransactionFixture struct {
	Sender              string `json:"sender"`
	Receiver            string `json:"receiver"`
	Nonce               uint64 `json:"nonce"`
	TransferredValue    uint64 `json:"transferredValue"`
	Tip                 uint64 `json:"tip"`
	Timestamp           uint64 `json:"timestamp"`
	TxHash              string `json:"txHash"`
	ThinkingMode        string `json:"thinkingMode"`
	UserOutputDimension string `json:"userOutputDimension"`
	Prompt              string `json:"prompt"`
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

	path := filepath.Join("testData", "miniround1_transactions.json")

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

func createTransactionFromFixture(fixture miniRoundOneTransactionFixture) data.Transaction {
	tx := mempool.NewTransaction()

	tx.SetSender([]byte(fixture.Sender))
	tx.SetReceiver([]byte(fixture.Receiver))
	tx.SetNonce(fixture.Nonce)
	tx.SetTransferredValue(fixture.TransferredValue)

	tx.SetPrompt([]byte(fixture.Prompt))
	tx.SetTip(fixture.Tip)
	tx.SetTimestamp(fixture.Timestamp)
	tx.SetTxHash([]byte(fixture.TxHash))

	tx.SetEstimatedFee(1)
	tx.SetThinkingMode(fixture.ThinkingMode)
	tx.SetUserOutputDimension(fixture.UserOutputDimension)

	return tx
}

func loadAgentLabelsFixtures(t *testing.T, numAgents int) []agentLabelsByTxHash {
	t.Helper()

	agents := make([]agentLabelsByTxHash, 0, numAgents)

	for i := 1; i <= numAgents; i++ {
		path := filepath.Join("testData", fmt.Sprintf("agent_%d.json", i))
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

func createAgentBackedLabeler(agentLabels agentLabelsByTxHash) agent.Agent {
	return &testscommon.LabelerStub{
		LabelCalled: func(tx data.Transaction) ([]string, error) {
			txHash := string(tx.GetTxHash())

			labels, ok := agentLabels.labelsByTxHash[txHash]
			if !ok {
				return nil, fmt.Errorf("agent %s has no labels for txHash %s", agentLabels.agent, txHash)
			}

			if len(labels) != 6 {
				return nil, fmt.Errorf(
					"agent %s has invalid labels count for txHash %s: got %d, expected 6",
					agentLabels.agent,
					txHash,
					len(labels),
				)
			}

			copiedLabels := copyStringSlice(labels)

			return copiedLabels, nil
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

			require.Lenf(t, labels, 6, "agent %s has invalid labels count for txHash %s", agentLabels.agent, txHash)

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

func consensusQuorum(numValidators int) int {
	return (2*numValidators)/3 + 1
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

	outputPath := filepath.Join("testData", "consensus_frequencies_results.jsonl")
	outputFile, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	require.NoError(t, err)
	defer outputFile.Close()

	_, err = outputFile.Write(append(encodedResult, '\n'))
	require.NoError(t, err)
}

func isSubset(subset []string, superset []string) bool {
	supersetMap := make(map[string]struct{}, len(superset))
	for _, item := range superset {
		supersetMap[item] = struct{}{}
	}

	for _, item := range subset {
		if _, ok := supersetMap[item]; !ok {
			return false
		}
	}

	return true
}
