package integrationtests

import (
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
	"moa-chain/consensus/miniround2"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

const integrationTestInitialBalance = uint64(10_000)

type integrationTestNode struct {
	id             string
	loop           *consensus.RoundLoop
	roundState     state.RoundState
	blockFinalizer *blockFinalizer.FinalizeBlockComponent
	logger         *logging.NodeLogger
}

func createValidators(pubKeys [][]byte) []*validators.Validator {
	vs := make([]*validators.Validator, 0, len(pubKeys))

	for i, pubkey := range pubKeys {
		v := validators.NewValidator(fmt.Sprintf("validator-%d", i+1), pubkey, 100)
		v.SetSubdomainScores(defaultIntegrationTestSubdomainScores())
		vs = append(vs, v)
	}

	return vs
}

func defaultIntegrationTestSubdomainScores() validators.SubdomainsScores {
	scores := make(validators.SubdomainsScores, len(data.PossibleSubDomains))
	for subdomain := range data.PossibleSubDomains {
		scores[subdomain] = 1
	}

	return scores
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
	batchAgent agent.BatchAgent,
	stopAfterMiniRoundOne bool,
	voteCollectionDeadline time.Duration,
) *integrationTestNode {
	peersRegistry := broadcast.NewPeerRegistry()
	for i, validator := range registeredValidators {
		err := peersRegistry.Register(validator.PublicID(), inboxes[i])
		require.NoError(t, err)
	}

	return createNodeWithBroadcaster(
		t,
		validatorID,
		privateKey,
		registeredValidators,
		myInbox,
		transactions,
		batchAgent,
		broadcast.NewBroadcaster(peersRegistry),
		stopAfterMiniRoundOne,
		voteCollectionDeadline,
	)
}

// createNodeWithBroadcaster allows scenario tests to control message delivery
// while reusing the same production handlers as the regular integration tests.
func createNodeWithBroadcaster(
	t *testing.T,
	validatorID string,
	privateKey []byte,
	registeredValidators []*validators.Validator,
	myInbox chan data.RoundEvent,
	transactions []data.Transaction,
	batchAgent agent.BatchAgent,
	broadcaster broadcast.Broadcaster,
	stopAfterMiniRoundOne bool,
	voteCollectionDeadline time.Duration,
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
	for _, tx := range transactions {
		err = txPool.AddTransaction(tx)
		require.NoError(t, err)
	}

	consensusSelector := validators.NewConsensusSelector(logger)
	validatorsRegistry := validators.NewValidatorRegistry(consensusSelector, logger)
	for _, validator := range registeredValidators {
		err = validatorsRegistry.Register(validator.PublicID(), validator)
		require.NoError(t, err)
	}

	roundState := state.NewRoundState()

	loop := createRoundLoop(
		validatorID,
		privateKey,
		txPool,
		validatorsRegistry,
		myInbox,
		finalizer,
		batchAgent,
		broadcaster,
		roundState,
		stopAfterMiniRoundOne,
		voteCollectionDeadline,
		logger,
	)

	require.NotNil(t, loop)

	return &integrationTestNode{
		id:             validatorID,
		loop:           loop,
		roundState:     roundState,
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
	validatorRegistry validators.ValidatorRegistry,
	inbox chan data.RoundEvent,
	blockFinalizer blockFinalizer.BlockFinalizer,
	batchAgent agent.BatchAgent,
	broadcaster broadcast.Broadcaster,
	roundState state.RoundState,
	stopAfterMiniRoundOne bool,
	voteCollectionDeadline time.Duration,
	logger *slog.Logger,
) *consensus.RoundLoop {
	currentHeader := currentIntegrationTestHeader()

	blockchainStateStub := &testscommon.BlockchainStateStub{
		CurrentBlockHeaderValue: currentHeader,
		CurrentRoundValue:       currentHeader.Round,
		CurrentMiniRoundValue:   currentHeader.MiniRound,
		CurrentEpochValue:       currentHeader.Epoch,
	}

	protocolAgent := integrationProtocolAgent{delegate: batchAgent}
	base := createBlockBase(txPool, blockchainStateStub, protocolAgent, logger)
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
		MyID:               nodeID,
		BlockProcessor:     validation.NewBlockProcessor(base),
		RoundState:         roundState,
		Broadcaster:        broadcaster,
		Signer:             signing.NewSigner(nodeID, privateKey),
		ValidatorRegistry:  validatorRegistry,
		BlockchainState:    blockchainStateStub,
		BlockFinalizer:     blockFinalizer,
		AnswerJudge:        protocolAgent,
		JudgeModelMetadata: "integration-deterministic-judge",
		Logger:             logger,
	}

	miniRoundTwoHandler := miniround2.NewMiniRoundTwoHandler(miniRoundTwoHandlerArgs)

	roundHandlerArgs := consensus.RoundHandlerArgs{
		SelfID:                 nodeID,
		CurrentStep:            data.StepIdle,
		CurrentRoundKey:        data.RoundKey{},
		MiniRoundOneHandler:    miniRoundOneHandler,
		MiniRoundTwoHandler:    miniRoundTwoHandler,
		BlockFinalizer:         blockFinalizer,
		StopAfterMiniRoundOne:  stopAfterMiniRoundOne,
		Logger:                 logger,
		Inbox:                  inbox,
		VoteCollectionDeadline: voteCollectionDeadline,
	}

	roundHandler := consensus.NewRoundHandler(roundHandlerArgs)

	return consensus.NewRoundLoop(roundHandler, inbox, logger)
}

// integrationProtocolAgent supplies deterministic non-empty answers and judge
// output so integration tests exercise the complete classification protocol.
type integrationProtocolAgent struct {
	delegate agent.BatchAgent
}

func (testAgent integrationProtocolAgent) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	return testAgent.delegate.LabelBatch(txs)
}

func (testAgent integrationProtocolAgent) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	results, err := testAgent.delegate.AnswerBatch(txs)
	if err != nil {
		return nil, err
	}

	for i, r := range results {
		if r.Answer == "" {
			results[i].Answer = "integration answer for " + string(r.TxHash)
		}
	}

	return results, nil
}

func (testAgent integrationProtocolAgent) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	response, err := testAgent.delegate.JudgeTransactionAnswers(request)
	if err == nil || !errors.Is(err, agent.ErrNotImplemented) {
		return response, err
	}

	var input struct {
		Candidates []struct {
			CandidateID string `json:"candidateId"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(request.UserPrompt), &input); err != nil {
		return "", err
	}

	output := struct {
		Classifications []struct {
			CandidateID string `json:"candidateId"`
			Category    string `json:"category"`
		} `json:"classifications"`
	}{Classifications: make([]struct {
		CandidateID string `json:"candidateId"`
		Category    string `json:"category"`
	}, len(input.Candidates))}
	for index, candidate := range input.Candidates {
		output.Classifications[index].CandidateID = candidate.CandidateID
		output.Classifications[index].Category = string(data.AnswerCategoryCorrect)
	}

	encoded, err := json.Marshal(output)
	return string(encoded), err
}

func createBlockBase(
	mempool mempool.Mempool,
	blockchainState state.BlockchainState,
	batchAgent agent.BatchAgent,
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
		BatchAgent:              batchAgent,
		AccountState:            accountStateStub,
		Mempool:                 mempool,
		Logger:                  logger,
	}
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
	"non_related":                        {},
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
func consensusQuorum(numValidators int) int {
	return (2*numValidators)/3 + 1
}
