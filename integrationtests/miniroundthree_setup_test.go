package integrationtests

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/proposing"
	"moa-chain/blockprocessing/validation"
	"moa-chain/broadcast"
	"moa-chain/consensus"
	"moa-chain/consensus/miniround1"
	"moa-chain/consensus/miniround2"
	"moa-chain/consensus/miniround3"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/txpipeline"
	"moa-chain/validators"
)

type mr3ScenarioCommittees struct {
	miniRoundOne   []string
	miniRoundTwo   []string
	miniRoundThree []string
}

func mr3ScenarioTransactions(scenario miniRoundThreeScenario) []data.Transaction {
	transactions := make([]data.Transaction, 0, len(scenario.Transactions))
	for _, fixture := range scenario.Transactions {
		transactions = append(transactions, createTransactionFromFixture(fixture.miniRoundOneTransactionFixture))
	}
	return transactions
}

func computeMR3SubdomainFrequencies(transactions []scenarioTransactionFixture, votes uint64) data.SubdomainsFrequency {
	frequencies := make(data.SubdomainsFrequency)
	for _, transaction := range transactions {
		for _, label := range transaction.Labels {
			frequencies[label] += votes
		}
	}
	return frequencies
}

func selectMR3ScenarioCommittees(
	t *testing.T,
	registeredValidators scenarioValidators,
	frequencies data.SubdomainsFrequency,
) mr3ScenarioCommittees {
	t.Helper()

	orderedValidators := orderedScenarioValidators(registeredValidators)
	blockchainState := &testscommon.BlockchainStateStub{CurrentBlockHeaderValue: currentIntegrationTestHeader()}

	mr1Key := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	mr2Key := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	mr3Key := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundThree)}

	mr1 := selectMiniRoundOneCommittee(t, blockchainState, orderedValidators, mr1Key)
	mr2 := selectMiniRoundTwoCommittee(t, blockchainState, orderedValidators, mr2Key, frequencies)
	mr3 := selectMR3Committee(t, blockchainState, orderedValidators, mr3Key, frequencies)

	return mr3ScenarioCommittees{miniRoundOne: mr1, miniRoundTwo: mr2, miniRoundThree: mr3}
}

func selectMR3Committee(
	t *testing.T,
	blockchainState *testscommon.BlockchainStateStub,
	orderedValidators scenarioValidators,
	roundKey data.RoundKey,
	frequencies data.SubdomainsFrequency,
) []string {
	t.Helper()

	selector := validators.NewConsensusSelector()
	committee, err := selector.SelectConsensusGroupMiniRoundThree(
		blockchainState, orderedValidators, roundKey, frequencies,
	)
	require.NoError(t, err)
	return committee
}

func createMiniRoundThreeScenarioNodes(
	t *testing.T,
	scenario miniRoundThreeScenario,
	registeredValidators scenarioValidators,
	privateKeys [][]byte,
	inboxes []chan data.RoundEvent,
	committees mr3ScenarioCommittees,
	network *testscommon.OrderedBroadcasterNetworkStub,
	transactions []data.Transaction,
) []*integrationTestNode {
	t.Helper()

	mr2RolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)
	mr3RolesByValidator := scenarioRolesByValidator(committees.miniRoundThree)

	nodes := make([]*integrationTestNode, 0, scenario.Network.RegisteredNodes)
	for index := 0; index < scenario.Network.RegisteredNodes; index++ {
		validatorID := fmt.Sprintf("validator-%d", index+1)

		mr2Role := mr2RolesByValidator[validatorID]
		if mr2Role == "" {
			mr2Role = "observer"
		}
		mr3Role := mr3RolesByValidator[validatorID]
		if mr3Role == "" {
			mr3Role = "observer"
		}

		node := createNodeWithMR3Broadcaster(
			t,
			validatorID,
			privateKeys[index],
			registeredValidators,
			inboxes[index],
			cloneTransactions(transactions),
			createMR3ScenarioAgent(t, scenario, mr2Role, mr3Role),
			network.BroadcasterForNode(validatorID),
		)
		nodes = append(nodes, node)
	}
	return nodes
}

func createNodeWithMR3Broadcaster(
	t *testing.T,
	validatorID string,
	privateKey []byte,
	registeredValidators scenarioValidators,
	myInbox chan data.RoundEvent,
	transactions []data.Transaction,
	batchAgent agent.BatchAgent,
	broadcaster broadcast.Broadcaster,
) *integrationTestNode {
	t.Helper()

	finalizer := blockFinalizer.NewFinalizeBlockComponent()
	nodeLogger, err := createIntegrationTestNodeLogger(t, validatorID)
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, nodeLogger.Close())
	})

	logger := nodeLogger.Logger()
	txPool := mempool.NewMemPool(logger)
	store := txpipeline.NewPrecomputedStore()
	txPreprocessor := txpipeline.NewTxPreprocessor(txpipeline.TxPreprocessorArgs{
		Agent:   integrationProtocolAgent{delegate: batchAgent},
		Store:   store,
		Mempool: txPool,
		Logger:  logger,
	})
	txInbox := make(chan data.Transaction, 128)
	txInterceptor := txpipeline.NewTxInterceptor(txpipeline.TxInterceptorArgs{
		Inbox:        txInbox,
		Preprocessor: txPreprocessor,
		Broadcaster:  broadcast.NewTxBroadcaster(broadcast.NewTxPeerRegistry()),
		SelfID:       validatorID,
		Logger:       logger,
	})

	txPreprocessor.Start()
	txInterceptor.Start()
	for _, tx := range transactions {
		err = txInterceptor.Submit(tx)
		require.NoError(t, err)
	}
	if len(transactions) > 0 {
		txPreprocessor.Stop()
		t.Cleanup(func() { txInterceptor.Stop() })
	} else {
		t.Cleanup(func() {
			txPreprocessor.Stop()
			txInterceptor.Stop()
		})
	}

	consensusSelector := validators.NewConsensusSelectorWithStrategy(validators.CommitteeStrategyHalf, logger)
	validatorsRegistry := validators.NewValidatorRegistry(consensusSelector, logger)
	for _, validator := range registeredValidators {
		err = validatorsRegistry.Register(validator.PublicID(), validator)
		require.NoError(t, err)
	}

	roundState := state.NewRoundState()

	loop := createRoundLoopWithMR3(
		validatorID,
		privateKey,
		txPool,
		validatorsRegistry,
		myInbox,
		finalizer,
		batchAgent,
		store,
		broadcaster,
		roundState,
		logger,
	)
	require.NotNil(t, loop)

	return &integrationTestNode{
		id:             validatorID,
		loop:           loop,
		roundState:     roundState,
		blockFinalizer: finalizer,
		logger:         nodeLogger,
		txInterceptor:  txInterceptor,
	}
}

func createRoundLoopWithMR3(
	nodeID string,
	privateKey []byte,
	txPool mempool.Mempool,
	validatorRegistry validators.ValidatorRegistry,
	inbox chan data.RoundEvent,
	blockFinalizer blockFinalizer.BlockFinalizer,
	batchAgent agent.BatchAgent,
	store txpipeline.PrecomputedStore,
	broadcaster broadcast.Broadcaster,
	roundState state.RoundState,
	logger *slog.Logger,
) *consensus.RoundLoop {
	currentHeader := currentIntegrationTestHeader()

	blockchainStateStub := &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: currentHeader,
		CurrentRoundValue:       currentHeader.Round,
		CurrentMiniRoundValue:   uint64(data.MiniRoundThree),
		CurrentEpochValue:       currentHeader.Epoch,
	}

	protocolAgent := integrationProtocolAgent{delegate: batchAgent}
	base := createBlockBase(txPool, blockchainStateStub, store, logger)

	miniRoundOneHandlerArgs := miniround1.MiniRoundOneHandlerArgs{
		MyID:              nodeID,
		BlockCreator:      proposing.NewBlockCreator(base),
		BlockProcessor:    validation.NewBlockProcessor(base),
		LabelsValidator:   validation.NewLabelsValidator(logger),
		RoundState:        roundState,
		Broadcaster:       broadcaster,
		Signer:            signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry: validatorRegistry,
		BlockchainState:   blockchainStateStub,
		BlockFinalizer:    blockFinalizer,
		Logger:            logger,
	}
	miniRoundOneHandler := miniround1.NewMiniRoundOneHandler(miniRoundOneHandlerArgs)

	miniRoundTwoHandlerArgs := miniround2.MiniRoundTwoHandlerArgs{
		MyID:                      nodeID,
		BlockProcessor:            validation.NewBlockProcessor(base),
		RoundState:                roundState,
		Broadcaster:               broadcaster,
		Signer:                    signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry:         validatorRegistry,
		BlockchainState:           blockchainStateStub,
		BlockFinalizer:            blockFinalizer,
		AnswerJudge:               protocolAgent,
		JudgeModelMetadata:        "integration-deterministic-judge",
		ClassificationGracePeriod: 0,
		Logger:                    logger,
		SelfInbox:                 inbox,
	}
	miniRoundTwoHandler := miniround2.NewMiniRoundTwoHandler(miniRoundTwoHandlerArgs)

	miniRoundThreeHandlerArgs := miniround3.MiniRoundThreeHandlerArgs{
		MyID:              nodeID,
		RoundState:        roundState,
		Broadcaster:       broadcaster,
		Signer:            signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry: validatorRegistry,
		BlockchainState:   blockchainStateStub,
		BlockFinalizer:    blockFinalizer,
		SynthesisAgent:    protocolAgent,
		Logger:            logger,
	}
	miniRoundThreeHandler := miniround3.NewMiniRoundThreeHandler(miniRoundThreeHandlerArgs)

	roundHandlerArgs := consensus.RoundHandlerArgs{
		SelfID:                 nodeID,
		CurrentStep:            data.StepIdle,
		CurrentRoundKey:        data.RoundKey{},
		MiniRoundOneHandler:    miniRoundOneHandler,
		MiniRoundTwoHandler:    miniRoundTwoHandler,
		MiniRoundThreeHandler:  miniRoundThreeHandler,
		BlockFinalizer:         blockFinalizer,
		StopAfterMiniRoundOne:  false,
		StopAfterMiniRoundTwo:  false,
		Logger:                 logger,
		Inbox:                  inbox,
		VoteCollectionDeadline: 0,
	}

	roundHandler := consensus.NewRoundHandler(roundHandlerArgs)
	return consensus.NewRoundLoop(roundHandler, inbox, logger)
}
