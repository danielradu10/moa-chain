package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

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

func TestMiniRoundOne_AllNodesFinalizeSameBlock(t *testing.T) {
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
) *integrationTestNode {
	txPool := mempool.NewMemPool()
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

	base := createBlockBase(txPool, blockchainStateStub)
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
		Labeler:                 &testscommon.LabelerStub{},
		AccountState:            accountStateStub,
		Mempool:                 mempool,
	}
}
