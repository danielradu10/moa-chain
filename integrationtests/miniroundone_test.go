package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
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
	"moa-chain/mempool"
	"moa-chain/state"
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
			if !node.blockFinalizer.WasFinalizeCalled() {
				return false
			}

			if node.blockFinalizer.GetFinalizedBlock() == nil {
				return false
			}
		}

		return true
	}, time.Second, 10*time.Millisecond)

	firstBlock := nodes[0].blockFinalizer.GetFinalizedBlock()
	require.NotNil(t, firstBlock)
	require.NotEmpty(t, firstBlock.Header.HeaderHash)

	for _, node := range nodes {
		finalizedBlock := node.blockFinalizer.GetFinalizedBlock()
		require.NotNil(t, finalizedBlock)

		require.Equal(t, firstBlock.Header.HeaderHash, finalizedBlock.Header.HeaderHash)
		require.Equal(t, firstBlock.Header.BodyHash, finalizedBlock.Header.BodyHash)
		require.Equal(t, firstBlock.Header.Nonce, finalizedBlock.Header.Nonce)
		require.Equal(t, firstBlock.Header.Round, finalizedBlock.Header.Round)
		require.Equal(t, firstBlock.Header.MiniRound, finalizedBlock.Header.MiniRound)
		require.Equal(t, firstBlock.Body.Subdomains, finalizedBlock.Body.Subdomains)
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
		LabelCalled: func(tx data.Transaction, amILeader bool) ([]string, error) {
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
			if !node.blockFinalizer.WasFinalizeCalled() {
				return false
			}

			if node.blockFinalizer.GetFinalizedBlock() == nil {
				return false
			}
		}

		return true
	}, time.Second, 10*time.Millisecond)

	firstBlock := nodes[0].blockFinalizer.GetFinalizedBlock()
	require.NotNil(t, firstBlock)
	require.NotEmpty(t, firstBlock.Header.HeaderHash)

	require.Len(t, firstBlock.Body.Transactions, len(transactions))
	require.Equal(t, expectedSubdomains(), firstBlock.Body.Subdomains)

	for _, node := range nodes {
		finalizedBlock := node.blockFinalizer.GetFinalizedBlock()
		require.NotNil(t, finalizedBlock)

		require.Equal(t, firstBlock.Header.HeaderHash, finalizedBlock.Header.HeaderHash)
		require.Equal(t, firstBlock.Header.BodyHash, finalizedBlock.Header.BodyHash)
		require.Equal(t, firstBlock.Header.Nonce, finalizedBlock.Header.Nonce)
		require.Equal(t, firstBlock.Header.Round, finalizedBlock.Header.Round)
		require.Equal(t, firstBlock.Header.MiniRound, finalizedBlock.Header.MiniRound)
		require.Equal(t, firstBlock.Body.Subdomains, finalizedBlock.Body.Subdomains)
		require.Len(t, finalizedBlock.Body.Transactions, len(transactions))
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
	blockFinalizer *testscommon.BlockFinalizerStub
}

func createValidators(pubKeys [][]byte) []*validators.Validator {
	vs := make([]*validators.Validator, 0, len(pubKeys))

	for i, pubkey := range pubKeys {
		v := validators.NewValidator(fmt.Sprintf("validator-%d", i+1), pubkey, 100)
		vs = append(vs, v)
	}

	return vs
}

func createNode(
	t *testing.T,
	validatorID string,
	privateKey []byte,
	registeredValidators []*validators.Validator,
	inboxes []chan data.RoundEvent,
	myInbox chan data.RoundEvent,
	transactions []data.Transaction,
	labeler agent.Labeler,
) *integrationTestNode {
	txPool := mempool.NewMemPool()

	for _, tx := range transactions {
		err := txPool.AddTransaction(tx)
		require.NoError(t, err)
	}

	peersRegistry := broadcast.NewPeerRegistry()
	consensusSelector := validators.NewConsensusSelector()
	validatorsRegistry := validators.NewValidatorRegistry(consensusSelector)

	for i, validator := range registeredValidators {
		err := validatorsRegistry.Register(validator.PublicID(), validator)
		require.NoError(t, err)

		err = peersRegistry.Register(validator.PublicID(), inboxes[i])
		require.NoError(t, err)
	}

	blockFinalizer := &testscommon.BlockFinalizerStub{}

	loop := createRoundLoop(
		validatorID,
		privateKey,
		txPool,
		peersRegistry,
		validatorsRegistry,
		myInbox,
		blockFinalizer,
		labeler,
	)

	require.NotNil(t, loop)

	return &integrationTestNode{
		id:             validatorID,
		loop:           loop,
		blockFinalizer: blockFinalizer,
	}
}

func createRoundLoop(
	nodeID string,
	privateKey []byte,
	txPool mempool.Mempool,
	peerRegistry broadcast.PeerRegistry,
	validatorRegistry validators.ValidatorRegistry,
	inbox chan data.RoundEvent,
	blockFinalizer blockFinalizer.BlockFinalizer,
	labeler agent.Labeler,
) *consensus.RoundLoop {
	currentHeader := &data.BlockHeader{
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

	blockchainStateStub := &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: currentHeader,
		CurrentRoundValue:       currentHeader.Round,
		CurrentMiniRoundValue:   currentHeader.MiniRound,
		CurrentEpochValue:       currentHeader.Epoch,
	}

	base := createBlockBase(txPool, blockchainStateStub, labeler)
	roundState := state.NewRoundState()

	miniRoundOneHandlerArgs := miniround1.MiniRoundOneHandlerArgs{
		MyID:              nodeID,
		BlockCreator:      proposing.NewBlockCreator(base),
		BlockValidator:    validation.NewBlockProcessor(base),
		RoundState:        roundState,
		Broadcaster:       broadcast.NewBroadcaster(peerRegistry),
		Signer:            signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry: validatorRegistry,
		BlockchainState:   blockchainStateStub,
		BlockFinalizer:    blockFinalizer,
	}

	miniRoundOneHandler := miniround1.NewMiniRoundOneHandler(miniRoundOneHandlerArgs)

	roundHandlerArgs := consensus.RoundHandlerArgs{
		SelfID:              nodeID,
		CurrentStep:         data.StepIdle,
		CurrentRoundKey:     data.RoundKey{},
		MiniRoundOneHandler: miniRoundOneHandler,
	}

	roundHandler := consensus.NewRoundHandler(roundHandlerArgs)

	return consensus.NewRoundLoop(roundHandler, inbox)
}

func createBlockBase(
	mempool mempool.Mempool,
	blockchainState state.BlockchainState,
	labelerCalled agent.Labeler,
) blockprocessing.Base {
	aliceAccount := testscommon.NewAccountHandlerStub(0, 100)
	bobAccount := testscommon.NewAccountHandlerStub(0, 100)
	carolAccount := testscommon.NewAccountHandlerStub(0, 100)
	davidAccount := testscommon.NewAccountHandlerStub(0, 100)
	evelineAccount := testscommon.NewAccountHandlerStub(0, 100)
	frankAccount := testscommon.NewAccountHandlerStub(0, 100)

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
	_ = accountStateStub.AddAccount("alice", 0, 100)
	_ = accountStateStub.AddAccount("bob", 0, 100)
	_ = accountStateStub.AddAccount("carol", 0, 100)
	_ = accountStateStub.AddAccount("david", 0, 100)
	_ = accountStateStub.AddAccount("eveline", 0, 100)
	_ = accountStateStub.AddAccount("frank", 0, 100)
	_ = accountStateStub.AddAccount("escrow", 0, 100)

	return blockprocessing.Base{
		AccountsSnapshotFactory: &accountSnapshotFactoryMock,
		BlockchainState:         blockchainState,
		Labeler:                 labelerCalled,
		AccountState:            accountStateStub,
		Mempool:                 mempool,
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

func expectedSubdomains() map[string]uint64 {
	return map[string]uint64{
		"back_end_with_apis":                 2,
		"databases":                          2,
		"security":                           2,
		"web_front_end":                      1,
		"test_engineering_and_qa_automation": 2,
		"data_engineering":                   1,
		"cloud_engineering":                  3,
		"blockchain_engineering":             1,
		"systems_programming":                1,
		"mobile_dev":                         1,
		"dev_ops":                            1,
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
