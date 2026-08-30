package localchain

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"log/slog"
	"path/filepath"
	"sync"
	"time"

	"moa-chain/agent"
	"moa-chain/agent/mock"
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/proposing"
	"moa-chain/blockprocessing/validation"
	"moa-chain/broadcast"
	"moa-chain/chain"
	"moa-chain/consensus"
	"moa-chain/consensus/miniround1"
	"moa-chain/consensus/miniround2"
	"moa-chain/consensus/miniround3"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/logging"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/tracker"
	"moa-chain/txpipeline"
	"moa-chain/validators"
)

// InitialBalance is the starting balance pre-funded for each known account.
const InitialBalance uint64 = 1_000_000

// Config holds parameters for creating a local chain.
type Config struct {
	NumNodes   int
	StartRound uint64
	AgentDelay time.Duration // simulated LLM latency per agent call; 0 = instant
	// MiniRoundDuration is a fixed slot enforced between mini-round transitions so
	// every mini-round takes at least this long regardless of whether there are
	// transactions. 0 means advance immediately (suitable for tests).
	MiniRoundDuration time.Duration
	// VoteCollectionDeadline lets an MR1 leader finalize with Q+ votes when a
	// full-committee member is missing. Zero retains the legacy all-G wait.
	VoteCollectionDeadline time.Duration
	// ClassificationGracePeriod keeps collecting valid MR2 classification
	// votes after Q is reached. Zero preserves immediate first-Q certification.
	ClassificationGracePeriod time.Duration
	// LogDir, when non-empty, writes each node's log to <LogDir>/<validatorID>.log
	// at INFO level while keeping the console at INFO. Leave empty to log all
	// nodes to the parent Logger only (suitable for tests).
	LogDir string
	Logger *slog.Logger

	// CommitteeStrategy controls how many validators are selected into consensus
	// committees. Defaults to CommitteeStrategyHalf when zero value.
	CommitteeStrategy validators.CommitteeStrategy
	// StopAfterMR2 prevents advancing to MR3 after MR2 finalization.
	StopAfterMR2 bool
	// Agents, when non-nil, provides one BatchAgent per validator (index matches
	// validator index). Indices beyond len(Agents), or nil entries, fall back to
	// a shared mock.BatchAgent with AgentDelay. Nil means all nodes use mock.
	Agents []agent.BatchAgent

	// ExtraHooks, when set, are composed with the internal round-tracker hooks
	// on node 0's block finalizer. Nil callbacks in ExtraHooks are ignored.
	ExtraHooks blockFinalizer.BlockFinalizerHooks
}

// Chain is a self-contained N-node simulator with a mocked LLM agent.
// Call Start to begin consensus and Stop to shut everything down.
type Chain struct {
	// NodeView is wired to node 0. Pass it to service.NewExplorerService to
	// build an HTTP server backed by the running chain.
	NodeView *explorer.NodeView

	cfg   Config
	nodes []*localNode
	dones []chan struct{}
	wg    sync.WaitGroup
}

// New creates all nodes and wires node 0's explorer components. Does not start
// any goroutines — call Start after wiring your HTTP handler.
func New(cfg Config) (*Chain, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	pubKeys := make([][]byte, cfg.NumNodes)
	privKeys := make([][]byte, cfg.NumNodes)
	for i := range pubKeys {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("key generation: %w", err)
		}
		pubKeys[i] = pub
		privKeys[i] = priv
	}

	vs := make([]*validators.Validator, cfg.NumNodes)
	for i, pub := range pubKeys {
		id := fmt.Sprintf("validator-%d", i+1)
		v := validators.NewValidator(id, pub, 1)
		v.SetSubdomainScores(allSubdomainScores())
		vs[i] = v
	}

	inboxes := make([]chan data.RoundEvent, cfg.NumNodes)
	for i := range inboxes {
		inboxes[i] = make(chan data.RoundEvent, 128)
	}

	peersRegistry := broadcast.NewPeerRegistry()
	for i, v := range vs {
		if err := peersRegistry.Register(v.PublicID(), inboxes[i]); err != nil {
			return nil, fmt.Errorf("peer registration: %w", err)
		}
	}
	broadcaster := broadcast.NewBroadcaster(peersRegistry)
	txPeerRegistry := broadcast.NewTxPeerRegistry()

	roundHub := explorer.NewRoundHub()
	txHub := explorer.NewTxHub()

	// Shared mock agent used as fallback when no per-validator agent is provided.
	sharedMock := &mock.BatchAgent{Delay: cfg.AgentDelay}
	agentFor := func(i int) agent.BatchAgent {
		if i < len(cfg.Agents) && cfg.Agents[i] != nil {
			return cfg.Agents[i]
		}
		return sharedMock
	}

	// CommitteeStrategyHalf is iota (0) — the zero value — so no defaulting needed.
	committeeStrategy := cfg.CommitteeStrategy

	nodes := make([]*localNode, cfg.NumNodes)
	for i, v := range vs {
		var onStepChanged func(data.RoundKey, data.Step)
		if i == 0 {
			onStepChanged = roundHub.OnStepChanged
		}

		var nodeLog *slog.Logger
		var nl *logging.NodeLogger
		if cfg.LogDir != "" {
			logPath := filepath.Join(cfg.LogDir, v.PublicID()+".log")
			created, err := logging.NewNodeLoggerWithLevels(v.PublicID(), logPath, slog.LevelInfo, slog.LevelInfo)
			if err != nil {
				return nil, fmt.Errorf("create logger for %s: %w", v.PublicID(), err)
			}
			nl = created
			nodeLog = nl.Logger()
		} else {
			nodeLog = cfg.Logger.With("node", v.PublicID())
		}

		nd, err := createNode(
			v.PublicID(),
			privKeys[i],
			vs,
			inboxes[i],
			agentFor(i),
			committeeStrategy,
			cfg.StopAfterMR2,
			broadcaster,
			txPeerRegistry,
			onStepChanged,
			cfg.MiniRoundDuration,
			cfg.VoteCollectionDeadline,
			cfg.ClassificationGracePeriod,
			nodeLog,
		)
		if err != nil {
			return nil, fmt.Errorf("create node %s: %w", v.PublicID(), err)
		}
		nd.nodeLogger = nl
		nodes[i] = nd
	}

	nodes[0].txTracker.OnStatusChanged = txHub.OnStatusChanged

	roundTracker := tracker.NewRoundTracker()
	node0Tracker := nodes[0].txTracker
	extraHooks := cfg.ExtraHooks
	nodes[0].finalizer.WithHooks(blockFinalizer.BlockFinalizerHooks{
		OnMR1Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
			roundTracker.OnMR1Finalized(key, block)
			if extraHooks.OnMR1Finalized != nil {
				extraHooks.OnMR1Finalized(key, block)
			}
		},
		OnMR2Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
			roundTracker.OnMR2Finalized(key, block)
			if extraHooks.OnMR2Finalized != nil {
				extraHooks.OnMR2Finalized(key, block)
			}
		},
		OnMR3Finalized: func(key data.RoundKey, block *data.BlockOnChain) {
			roundTracker.OnMR3Finalized(key, block)
			for _, tx := range block.Body.Transactions {
				node0Tracker.OnFinalized(string(tx.GetTxHash()))
			}
			if extraHooks.OnMR3Finalized != nil {
				extraHooks.OnMR3Finalized(key, block)
			}
		},
	})

	nodeView := &explorer.NodeView{
		Chain:           nodes[0].nodeChain,
		BlockchainState: nodes[0].blockchainState,
		BlockFinalizer:  nodes[0].finalizer,
		Store:           nodes[0].store,
		Mempool:         nodes[0].mempool,
		TxTracker:       nodes[0].txTracker,
		RoundTracker:    roundTracker,
		RoundLoop:       nodes[0].loop,
		RoundHub:        roundHub,
		TxHub:           txHub,
		TxSubmitter:     nodes[0].txInterceptor,
	}

	return &Chain{
		NodeView: nodeView,
		cfg:      cfg,
		nodes:    nodes,
	}, nil
}

// Start launches all node consensus loops and sends the initial StartRoundEvent.
// Must be called exactly once.
func (c *Chain) Start() {
	c.dones = make([]chan struct{}, len(c.nodes))
	for i, n := range c.nodes {
		done := make(chan struct{})
		c.dones[i] = done
		c.wg.Add(1)
		go func(nd *localNode, d chan struct{}) {
			defer c.wg.Done()
			defer close(d)
			nd.loop.Run()
		}(n, done)
	}

	firstRoundKey := data.RoundKey{
		Epoch:     0,
		Round:     c.cfg.StartRound,
		MiniRound: uint64(data.MiniRoundOne),
	}
	for _, n := range c.nodes {
		n.inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: firstRoundKey}
	}
}

// Close closes all per-node log files. Call after Stop.
func (c *Chain) Close() {
	for _, n := range c.nodes {
		if n.nodeLogger != nil {
			_ = n.nodeLogger.Close()
		}
	}
}

// Stop shuts down all node loops and tx pipeline components. Safe to call if
// Start was never called.
func (c *Chain) Stop() {
	if c.dones == nil {
		return
	}
	for i, n := range c.nodes {
		n.loop.Shutdown(c.dones[i])
	}
	c.wg.Wait()
	for _, n := range c.nodes {
		n.stopPreprocessor()
		n.stopInterceptor()
	}
}

// localNode holds all components for one validator.
type localNode struct {
	id               string
	loop             *consensus.RoundLoop
	finalizer        *blockFinalizer.FinalizeBlockComponent
	nodeChain        chain.Chain
	blockchainState  state.BlockchainState
	txInterceptor    txpipeline.TxInterceptor
	txTracker        *tracker.TxTracker
	store            txpipeline.PrecomputedStore
	mempool          mempool.Mempool
	inbox            chan data.RoundEvent
	nodeLogger       *logging.NodeLogger // non-nil when LogDir is set
	stopPreprocessor func()
	stopInterceptor  func()
}

func createNode(
	id string,
	privateKey []byte,
	allValidators []*validators.Validator,
	inbox chan data.RoundEvent,
	agentImpl agent.BatchAgent,
	committeeStrategy validators.CommitteeStrategy,
	stopAfterMR2 bool,
	broadcaster broadcast.Broadcaster,
	txPeerRegistry broadcast.TxPeerRegistry,
	onStepChanged func(data.RoundKey, data.Step),
	miniRoundDuration time.Duration,
	voteCollectionDeadline time.Duration,
	classificationGracePeriod time.Duration,
	logger *slog.Logger,
) (*localNode, error) {
	finalizer := blockFinalizer.NewFinalizeBlockComponent()
	txPool := mempool.NewMemPool(logger)
	store := txpipeline.NewPrecomputedStore()
	txTrack := tracker.NewTxTracker()

	txPreprocessor := txpipeline.NewTxPreprocessor(txpipeline.TxPreprocessorArgs{
		Agent:             agentImpl,
		Store:             store,
		Mempool:           txPool,
		Logger:            logger,
		OnTxPreprocessing: txTrack.OnPreprocessing,
		OnTxPending:       txTrack.OnPending,
	})

	txInbox := make(chan data.Transaction, 128)
	if err := txPeerRegistry.Register(id, txInbox); err != nil {
		return nil, fmt.Errorf("tx peer registration: %w", err)
	}

	txInt := txpipeline.NewTxInterceptor(txpipeline.TxInterceptorArgs{
		Inbox:         txInbox,
		Preprocessor:  txPreprocessor,
		Broadcaster:   broadcast.NewTxBroadcaster(txPeerRegistry),
		SelfID:        id,
		Logger:        logger,
		OnTxSubmitted: txTrack.OnSubmitted,
	})

	txPreprocessor.Start()
	txInt.Start()

	selector := validators.NewConsensusSelectorWithStrategy(committeeStrategy, logger)
	registry := validators.NewValidatorRegistry(selector, logger)
	for _, v := range allValidators {
		if err := registry.Register(v.PublicID(), v); err != nil {
			return nil, fmt.Errorf("validator registration: %w", err)
		}
	}

	roundState := state.NewRoundState()
	nodeChain := chain.NewChain()
	if err := nodeChain.Append(&data.BlockOnChain{Header: genesisHeader()}); err != nil {
		return nil, fmt.Errorf("genesis block: %w", err)
	}

	blockchainState := state.NewBlockchainState(nodeChain)
	genesis := genesisHeader()
	blockchainState.Update(genesis.Round, data.MiniRoundThree, genesis.Epoch)

	base := makeBlockBase(txPool, blockchainState, store, logger)

	mr1Handler := miniround1.NewMiniRoundOneHandler(miniround1.MiniRoundOneHandlerArgs{
		MyID:              id,
		BlockCreator:      proposing.NewBlockCreator(base),
		BlockProcessor:    validation.NewBlockProcessor(base),
		LabelsValidator:   validation.NewLabelsValidator(logger),
		RoundState:        roundState,
		Broadcaster:       broadcaster,
		Signer:            signing.NewSigner(id, privateKey),
		ValidatorRegistry: registry,
		BlockchainState:   blockchainState,
		BlockFinalizer:    finalizer,
		Logger:            logger,
	})

	mr2Handler := miniround2.NewMiniRoundTwoHandler(miniround2.MiniRoundTwoHandlerArgs{
		MyID:                      id,
		BlockProcessor:            validation.NewBlockProcessor(base),
		RoundState:                roundState,
		Broadcaster:               broadcaster,
		Signer:                    signing.NewSigner(id, privateKey),
		ValidatorRegistry:         registry,
		BlockchainState:           blockchainState,
		BlockFinalizer:            finalizer,
		AnswerJudge:               agentImpl,
		JudgeModelMetadata:        "mock-judge",
		ClassificationGracePeriod: classificationGracePeriod,
		Logger:                    logger,
		SelfInbox:                 inbox,
	})

	mr3Handler := miniround3.NewMiniRoundThreeHandler(miniround3.MiniRoundThreeHandlerArgs{
		MyID:              id,
		RoundState:        roundState,
		Broadcaster:       broadcaster,
		Signer:            signing.NewSigner(id, privateKey),
		ValidatorRegistry: registry,
		BlockchainState:   blockchainState,
		BlockFinalizer:    finalizer,
		SynthesisAgent:    agentImpl,
		Logger:            logger,
	})

	roundHandler := consensus.NewRoundHandler(consensus.RoundHandlerArgs{
		SelfID:                 id,
		CurrentStep:            data.StepIdle,
		CurrentRoundKey:        data.RoundKey{},
		MiniRoundOneHandler:    mr1Handler,
		MiniRoundTwoHandler:    mr2Handler,
		MiniRoundThreeHandler:  mr3Handler,
		BlockFinalizer:         finalizer,
		Chain:                  nodeChain,
		Mempool:                txPool,
		Store:                  store,
		RoundState:             roundState,
		BlockchainState:        blockchainState,
		Logger:                 logger,
		Inbox:                  inbox,
		MiniRoundDuration:      miniRoundDuration,
		VoteCollectionDeadline: voteCollectionDeadline,
		OnStepChanged:          onStepChanged,
		StopAfterMiniRoundTwo:  stopAfterMR2,
	})

	loop := consensus.NewRoundLoop(roundHandler, inbox, logger)

	return &localNode{
		id:               id,
		loop:             loop,
		finalizer:        finalizer,
		nodeChain:        nodeChain,
		blockchainState:  blockchainState,
		txInterceptor:    txInt,
		txTracker:        txTrack,
		store:            store,
		mempool:          txPool,
		inbox:            inbox,
		stopPreprocessor: txPreprocessor.Stop,
		stopInterceptor:  txInt.Stop,
	}, nil
}

func genesisHeader() data.ChainBlockHeader {
	return data.ChainBlockHeader{
		BodyHash:         []byte("body hash 1"),
		HeaderHash:       []byte("header hash 1"),
		PreviousHash:     []byte("previous hash 0"),
		RootHash:         []byte("root hash 1"),
		PreviousRootHash: []byte("previous root hash 0"),
		Nonce:            1,
		Round:            1,
		Epoch:            0,
	}
}

func makeBlockBase(
	mp mempool.Mempool,
	blockchainState state.BlockchainState,
	store txpipeline.PrecomputedStore,
	logger *slog.Logger,
) blockprocessing.Base {
	accounts := map[string]*testscommon.AccountHandlerStub{
		"alice":   testscommon.NewAccountHandlerStub(0, InitialBalance),
		"bob":     testscommon.NewAccountHandlerStub(0, InitialBalance),
		"carol":   testscommon.NewAccountHandlerStub(0, InitialBalance),
		"david":   testscommon.NewAccountHandlerStub(0, InitialBalance),
		"eveline": testscommon.NewAccountHandlerStub(0, InitialBalance),
		"frank":   testscommon.NewAccountHandlerStub(0, InitialBalance),
	}
	escrow := testscommon.NewAccountHandlerStub(0, 0)

	snapshot := &testscommon.AccountsSnapshotStub{
		Accounts:      accounts,
		EscrowAccount: escrow,
	}
	snapshotFactory := testscommon.AccountsSnapshotFactoryStub{Snapshot: snapshot}

	accountState := testscommon.NewAccountStateStub()
	for name := range accounts {
		_ = accountState.AddAccount(name, 0, InitialBalance)
	}
	_ = accountState.AddAccount("escrow", 0, 0)

	return blockprocessing.Base{
		AccountsSnapshotFactory: &snapshotFactory,
		BlockchainState:         blockchainState,
		Store:                   store,
		AccountState:            accountState,
		Mempool:                 mp,
		Logger:                  logger,
	}
}

func allSubdomainScores() validators.SubdomainsScores {
	scores := make(validators.SubdomainsScores, len(data.PossibleSubDomains))
	for sub := range data.PossibleSubDomains {
		scores[sub] = 1
	}
	return scores
}
