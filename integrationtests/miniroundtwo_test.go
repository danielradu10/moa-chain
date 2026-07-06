package integrationtests

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

// Add a scenario name here to run another fixture through the same MR1 -> MR2
// protocol runner. Scenario-specific behavior belongs in its fixture, not in
// this test function.
func TestMiniRoundOneToMiniRoundTwoScenarios(t *testing.T) {
	scenarios := []string{
		"unanimous_correct",
	}

	for _, scenarioName := range scenarios {
		t.Run(scenarioName, func(t *testing.T) {
			runMiniRoundTwoScenario(t, loadMiniRoundTwoScenario(t, scenarioName))
		})
	}
}

type miniRoundTwoScenario struct {
	Name         string                       `json:"name"`
	Description  string                       `json:"description"`
	Network      scenarioNetworkFixture       `json:"network"`
	Transactions []scenarioTransactionFixture `json:"transactions"`
	Executors    []scenarioExecutorFixture    `json:"executors"`
	Judges       []scenarioJudgeFixture       `json:"judges"`
	Delivery     scenarioDeliveryFixture      `json:"delivery"`
	Expected     scenarioExpectedFixture      `json:"expected"`
	directory    string
}

type scenarioNetworkFixture struct {
	RegisteredNodes int `json:"registeredNodes"`
	CommitteeSize   int `json:"committeeSize"`
	Quorum          int `json:"quorum"`
}

type scenarioTransactionFixture struct {
	miniRoundOneTransactionFixture
	Role   string   `json:"role"`
	Labels []string `json:"labels"`
}

type scenarioExecutorFixture struct {
	Role    string            `json:"role"`
	Answers map[string]string `json:"answers"`
}

type scenarioJudgeFixture struct {
	Role            string                               `json:"role"`
	Mode            string                               `json:"mode"`
	DefaultCategory data.AnswerCategory                  `json:"defaultCategory"`
	Classifications []scenarioJudgeClassificationFixture `json:"classifications,omitempty"`
}

type scenarioJudgeClassificationFixture struct {
	TxHash   string              `json:"txHash"`
	Answer   string              `json:"answer"`
	Category data.AnswerCategory `json:"category"`
}

type scenarioDeliveryFixture struct {
	ProducerOrder []string `json:"producerOrder"`
	JudgeOrder    []string `json:"judgeOrder"`
}

type scenarioExpectedFixture struct {
	RoundFinalized          bool                          `json:"roundFinalized"`
	FinalizedNodes          int                           `json:"finalizedNodes"`
	AnswerEvidenceProducers []string                      `json:"answerEvidenceProducers"`
	ClassificationVoters    []string                      `json:"classificationVoters"`
	Transactions            []scenarioExpectedTransaction `json:"transactions"`
}

type scenarioExpectedTransaction struct {
	TxHash          string                         `json:"txHash"`
	Answers         int                            `json:"answers"`
	CountsPerAnswer scenarioExpectedCategoryCounts `json:"countsPerAnswer"`
	Groups          scenarioExpectedGroupSizes     `json:"groups"`
	Status          data.TransactionAnswerStatus   `json:"status"`
}

type scenarioExpectedGroupSizes struct {
	Correct       int `json:"correct"`
	Hallucination int `json:"hallucination"`
	Malicious     int `json:"malicious"`
	Wrong         int `json:"wrong"`
}

type scenarioExpectedCategoryCounts struct {
	Correct       uint64 `json:"correct"`
	Hallucination uint64 `json:"hallucination"`
	Malicious     uint64 `json:"malicious"`
	Wrong         uint64 `json:"wrong"`
}

type scenarioCommittees struct {
	miniRoundOne []string
	miniRoundTwo []string
}

func loadMiniRoundTwoScenario(t *testing.T, name string) miniRoundTwoScenario {
	t.Helper()

	directory := filepath.Join("testData", "miniround2", "scenarios", name)
	rawFixture, err := os.ReadFile(filepath.Join(directory, "scenario.json"))
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(rawFixture))
	decoder.DisallowUnknownFields()

	var scenario miniRoundTwoScenario
	require.NoError(t, decoder.Decode(&scenario))
	require.Equal(t, name, scenario.Name)
	scenario.directory = directory

	validateMiniRoundTwoScenario(t, scenario)
	return scenario
}

func validateMiniRoundTwoScenario(t *testing.T, scenario miniRoundTwoScenario) {
	t.Helper()

	require.NotEmpty(t, scenario.Description)
	require.Equal(t, 20, scenario.Network.RegisteredNodes)
	require.Equal(t, scenario.Network.RegisteredNodes/2, scenario.Network.CommitteeSize)
	require.Equal(t, consensusQuorum(scenario.Network.CommitteeSize), scenario.Network.Quorum)
	require.Len(t, scenario.Transactions, 3)
	require.True(t, scenario.Expected.RoundFinalized)
	require.Equal(t, scenario.Network.RegisteredNodes, scenario.Expected.FinalizedNodes)
	require.Len(t, scenario.Expected.AnswerEvidenceProducers, scenario.Network.Quorum)
	require.Len(t, scenario.Expected.ClassificationVoters, scenario.Network.Quorum)
	require.Len(t, scenario.Expected.Transactions, len(scenario.Transactions))

	txHashes := make(map[string]struct{}, len(scenario.Transactions))
	for _, transaction := range scenario.Transactions {
		require.NotEmpty(t, transaction.Role)
		require.NotEmpty(t, transaction.TxHash)
		require.NotEmpty(t, transaction.Prompt)
		_, duplicated := txHashes[transaction.TxHash]
		require.Falsef(t, duplicated, "duplicated transaction %s", transaction.TxHash)
		txHashes[transaction.TxHash] = struct{}{}

		require.Len(t, transaction.Labels, 6)
		seenLabels := make(map[string]struct{}, len(transaction.Labels))
		for _, label := range transaction.Labels {
			_, valid := possibleSubDomains[label]
			require.Truef(t, valid, "invalid label %q for transaction %s", label, transaction.TxHash)
			_, duplicated = seenLabels[label]
			require.Falsef(t, duplicated, "duplicated label %q for transaction %s", label, transaction.TxHash)
			seenLabels[label] = struct{}{}
		}
	}

	validateScenarioProfiles(t, scenario, txHashes)
	validateScenarioRoleOrder(t, scenario.Delivery.ProducerOrder, scenario.Network.CommitteeSize)
	validateScenarioRoleOrder(t, scenario.Delivery.JudgeOrder, scenario.Network.CommitteeSize)

	expectedTxHashes := make(map[string]struct{}, len(scenario.Expected.Transactions))
	for _, expected := range scenario.Expected.Transactions {
		_, exists := txHashes[expected.TxHash]
		require.Truef(t, exists, "expected result references unknown transaction %s", expected.TxHash)
		_, duplicated := expectedTxHashes[expected.TxHash]
		require.Falsef(t, duplicated, "duplicated expected transaction %s", expected.TxHash)
		expectedTxHashes[expected.TxHash] = struct{}{}
		require.Equal(t, scenario.Network.Quorum, expected.Answers)
		require.Equal(
			t,
			expected.Answers,
			expected.Groups.Correct+expected.Groups.Hallucination+expected.Groups.Malicious+expected.Groups.Wrong,
		)
		require.True(t, expected.Status.IsValid())
	}
}

func validateScenarioProfiles(t *testing.T, scenario miniRoundTwoScenario, txHashes map[string]struct{}) {
	t.Helper()

	executorRoles := make(map[string]struct{}, len(scenario.Executors))
	for _, executor := range scenario.Executors {
		require.NotEmpty(t, executor.Role)
		_, duplicated := executorRoles[executor.Role]
		require.Falsef(t, duplicated, "duplicated executor role %s", executor.Role)
		executorRoles[executor.Role] = struct{}{}

		for txHash := range txHashes {
			answer, exists := executor.Answers[txHash]
			require.Truef(t, exists, "executor %s has no answer for %s", executor.Role, txHash)
			require.NotEmpty(t, answer)
		}
	}
	_, hasDefaultExecutor := executorRoles["default"]
	require.True(t, hasDefaultExecutor)

	judgeRoles := make(map[string]struct{}, len(scenario.Judges))
	for _, judge := range scenario.Judges {
		require.NotEmpty(t, judge.Role)
		_, duplicated := judgeRoles[judge.Role]
		require.Falsef(t, duplicated, "duplicated judge role %s", judge.Role)
		judgeRoles[judge.Role] = struct{}{}
		require.Equal(t, "valid", judge.Mode)
		require.True(t, judge.DefaultCategory.IsValid())
		for _, classificationFixture := range judge.Classifications {
			_, exists := txHashes[classificationFixture.TxHash]
			require.Truef(t, exists, "judge %s references unknown transaction %s", judge.Role, classificationFixture.TxHash)
			require.NotEmpty(t, classificationFixture.Answer)
			require.True(t, classificationFixture.Category.IsValid())
		}
	}
	_, hasDefaultJudge := judgeRoles["default"]
	require.True(t, hasDefaultJudge)
}

func validateScenarioRoleOrder(t *testing.T, roles []string, committeeSize int) {
	t.Helper()
	require.Len(t, roles, committeeSize)

	seen := make(map[string]struct{}, len(roles))
	for index, role := range roles {
		expectedRole := scenarioCommitteeRole(index)
		require.Equal(t, expectedRole, role)
		_, duplicated := seen[role]
		require.False(t, duplicated)
		seen[role] = struct{}{}
	}
}

func runMiniRoundTwoScenario(t *testing.T, scenario miniRoundTwoScenario) {
	t.Helper()

	publicKeys, privateKeys := generateScenarioKeys(t, scenario.Network.RegisteredNodes)
	registeredValidators := createScenarioValidators(publicKeys)
	transactions := scenarioTransactions(scenario)
	frequencies := scenarioSubdomainFrequencies(scenario, uint64(scenario.Network.Quorum))
	committees := selectScenarioCommittees(t, registeredValidators, frequencies)
	require.Len(t, committees.miniRoundOne, scenario.Network.CommitteeSize)
	require.Len(t, committees.miniRoundTwo, scenario.Network.CommitteeSize)

	miniRoundTwoRoles := scenarioRolesByValidator(committees.miniRoundTwo)
	inboxes := makeScenarioInboxes(scenario.Network.RegisteredNodes)
	network := newScenarioNetwork(
		inboxes,
		scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Delivery.ProducerOrder),
		scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Delivery.JudgeOrder),
	)

	nodes := make([]*integrationTestNode, 0, scenario.Network.RegisteredNodes)
	for index := 0; index < scenario.Network.RegisteredNodes; index++ {
		validatorID := fmt.Sprintf("validator-%d", index+1)
		role := miniRoundTwoRoles[validatorID]
		if role == "" {
			role = "observer"
		}

		node := createNodeWithBroadcaster(
			t,
			validatorID,
			privateKeys[index],
			registeredValidators,
			inboxes[index],
			cloneTransactions(transactions),
			createScenarioAgent(t, scenario, role),
			&scenarioBroadcaster{nodeID: validatorID, network: network},
		)
		nodes = append(nodes, node)
	}

	done := startScenarioNodes(nodes)
	var stopOnce sync.Once
	stopNodes := func() {
		stopOnce.Do(func() {
			for _, inbox := range inboxes {
				inbox <- data.RoundEvent{Type: data.StopEvent}
			}
			for _, nodeDone := range done {
				select {
				case <-nodeDone:
				case <-time.After(5 * time.Second):
					t.Errorf("timed out stopping scenario node")
				}
			}
		})
	}
	t.Cleanup(stopNodes)

	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{Type: data.StartRoundEvent, RoundKey: miniRoundOneKey}
	}

	var protocolErr error
	require.Eventually(t, func() bool {
		if protocolErr = firstScenarioError(nodes); protocolErr != nil {
			return true
		}

		for _, node := range nodes {
			if _, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey); err != nil {
				return false
			}
		}
		return true
	}, 10*time.Second, 10*time.Millisecond)
	require.NoError(t, protocolErr)

	stopNodes()
	require.NoError(t, firstScenarioError(nodes))

	firstMiniRoundOneBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
	require.NoError(t, err)
	firstMiniRoundTwoBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
	require.NoError(t, err)

	leaderID := committees.miniRoundTwo[0]
	leaderNode := scenarioNodeByID(t, nodes, leaderID)
	classificationVotes, err := leaderNode.roundState.GetAnswerClassificationVotes(miniRoundTwoKey)
	require.NoError(t, err)

	requireScenarioFinalizedState(
		t,
		scenario,
		nodes,
		committees,
		frequencies,
		firstMiniRoundOneBlock,
		firstMiniRoundTwoBlock,
		classificationVotes,
		miniRoundOneKey,
		miniRoundTwoKey,
	)

	writeScenarioResult(t, scenario, committees, firstMiniRoundOneBlock, firstMiniRoundTwoBlock, classificationVotes)
}

func generateScenarioKeys(t *testing.T, count int) ([][]byte, [][]byte) {
	t.Helper()
	publicKeys := make([][]byte, 0, count)
	privateKeys := make([][]byte, 0, count)
	for index := 0; index < count; index++ {
		publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
		require.NoError(t, err)
		publicKeys = append(publicKeys, publicKey)
		privateKeys = append(privateKeys, privateKey)
	}
	return publicKeys, privateKeys
}

func createScenarioValidators(publicKeys [][]byte) []*validators.Validator {
	result := make([]*validators.Validator, 0, len(publicKeys))
	for index, publicKey := range publicKeys {
		validator := validators.NewValidator(fmt.Sprintf("validator-%d", index+1), publicKey, 1)
		validator.SetSubdomainScores(defaultIntegrationTestSubdomainScores())
		result = append(result, validator)
	}
	return result
}

func scenarioTransactions(scenario miniRoundTwoScenario) []data.Transaction {
	transactions := make([]data.Transaction, 0, len(scenario.Transactions))
	for _, fixture := range scenario.Transactions {
		transactions = append(transactions, createTransactionFromFixture(fixture.miniRoundOneTransactionFixture))
	}
	return transactions
}

func scenarioSubdomainFrequencies(scenario miniRoundTwoScenario, votes uint64) data.SubdomainsFrequency {
	frequencies := make(data.SubdomainsFrequency)
	for _, transaction := range scenario.Transactions {
		for _, label := range transaction.Labels {
			frequencies[label] += votes
		}
	}
	return frequencies
}

func selectScenarioCommittees(
	t *testing.T,
	registeredValidators []*validators.Validator,
	frequencies data.SubdomainsFrequency,
) scenarioCommittees {
	t.Helper()

	// ValidatorRegistry canonicalizes validators by public ID before invoking the
	// selector. Preselection must use that same ordering or fixture roles would
	// target a different committee than the protocol actually selects.
	orderedValidators := append([]*validators.Validator(nil), registeredValidators...)
	sort.Slice(orderedValidators, func(left, right int) bool {
		return orderedValidators[left].PublicID() < orderedValidators[right].PublicID()
	})

	blockchainState := &testscommon.BlockchainStateStub{CurrentBlockHeaderValue: currentIntegrationTestHeader()}
	miniRoundOneKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
	miniRoundTwoKey := data.RoundKey{Epoch: 0, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}

	miniRoundOneSelector := validators.NewConsensusSelector()
	miniRoundOne, err := miniRoundOneSelector.SelectConsensusGroupMiniRoundOne(
		blockchainState,
		orderedValidators,
		miniRoundOneKey,
	)
	require.NoError(t, err)

	miniRoundTwoSelector := validators.NewConsensusSelector()
	miniRoundTwo, err := miniRoundTwoSelector.SelectConsensusGroupMiniRoundTwo(
		blockchainState,
		orderedValidators,
		miniRoundTwoKey,
		frequencies,
	)
	require.NoError(t, err)

	return scenarioCommittees{miniRoundOne: miniRoundOne, miniRoundTwo: miniRoundTwo}
}

func scenarioCommitteeRole(index int) string {
	if index == 0 {
		return "leader"
	}
	return fmt.Sprintf("member-%d", index)
}

func scenarioRolesByValidator(committee []string) map[string]string {
	roles := make(map[string]string, len(committee))
	for index, validatorID := range committee {
		roles[validatorID] = scenarioCommitteeRole(index)
	}
	return roles
}

func scenarioIDsForRoles(t *testing.T, committee []string, roles []string) []string {
	t.Helper()
	ids := make([]string, 0, len(roles))
	for _, role := range roles {
		if role == "leader" {
			ids = append(ids, committee[0])
			continue
		}

		var memberIndex int
		_, err := fmt.Sscanf(role, "member-%d", &memberIndex)
		require.NoError(t, err)
		require.GreaterOrEqual(t, memberIndex, 1)
		require.Less(t, memberIndex, len(committee))
		ids = append(ids, committee[memberIndex])
	}
	return ids
}

func makeScenarioInboxes(count int) []chan data.RoundEvent {
	inboxes := make([]chan data.RoundEvent, 0, count)
	for index := 0; index < count; index++ {
		inboxes = append(inboxes, make(chan data.RoundEvent, 256))
	}
	return inboxes
}

func createScenarioAgent(t *testing.T, scenario miniRoundTwoScenario, role string) agent.Agent {
	t.Helper()

	labelsByTxHash := make(map[string][]string, len(scenario.Transactions))
	for _, transaction := range scenario.Transactions {
		labelsByTxHash[transaction.TxHash] = copyStringSlice(transaction.Labels)
	}

	executor := scenarioExecutorForRole(t, scenario.Executors, role)
	judge := scenarioJudgeForRole(t, scenario.Judges, role)
	return &testscommon.LabelerStub{
		LabelsByTxHash:  labelsByTxHash,
		AnswersByTxHash: executor.Answers,
		JudgeAnswersCalled: func(request agent.AnswerJudgeRequest) (string, error) {
			return executeScenarioJudge(request, judge)
		},
	}
}

func scenarioExecutorForRole(
	t *testing.T,
	profiles []scenarioExecutorFixture,
	role string,
) scenarioExecutorFixture {
	t.Helper()
	defaultProfile := scenarioExecutorFixture{}
	for _, profile := range profiles {
		if profile.Role == role {
			return profile
		}
		if profile.Role == "default" {
			defaultProfile = profile
		}
	}
	require.NotEmpty(t, defaultProfile.Role)
	return defaultProfile
}

func scenarioJudgeForRole(
	t *testing.T,
	profiles []scenarioJudgeFixture,
	role string,
) scenarioJudgeFixture {
	t.Helper()
	defaultProfile := scenarioJudgeFixture{}
	for _, profile := range profiles {
		if profile.Role == role {
			return profile
		}
		if profile.Role == "default" {
			defaultProfile = profile
		}
	}
	require.NotEmpty(t, defaultProfile.Role)
	return defaultProfile
}

func executeScenarioJudge(request agent.AnswerJudgeRequest, judge scenarioJudgeFixture) (string, error) {
	var input struct {
		TransactionHash string `json:"transactionHash"`
		Candidates      []struct {
			CandidateID string `json:"candidateId"`
			Answer      string `json:"answer"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(request.UserPrompt), &input); err != nil {
		return "", err
	}

	txHashBytes, err := hex.DecodeString(input.TransactionHash)
	if err != nil {
		return "", err
	}
	txHash := string(txHashBytes)

	output := struct {
		Classifications []struct {
			CandidateID string              `json:"candidateId"`
			Category    data.AnswerCategory `json:"category"`
		} `json:"classifications"`
	}{Classifications: make([]struct {
		CandidateID string              `json:"candidateId"`
		Category    data.AnswerCategory `json:"category"`
	}, 0, len(input.Candidates))}

	for _, candidate := range input.Candidates {
		category := judge.DefaultCategory
		for _, configured := range judge.Classifications {
			if configured.TxHash == txHash && configured.Answer == candidate.Answer {
				category = configured.Category
				break
			}
		}
		output.Classifications = append(output.Classifications, struct {
			CandidateID string              `json:"candidateId"`
			Category    data.AnswerCategory `json:"category"`
		}{CandidateID: candidate.CandidateID, Category: category})
	}

	encoded, err := json.Marshal(output)
	return string(encoded), err
}

type scenarioNetwork struct {
	mutex               sync.Mutex
	inboxes             map[string]chan data.RoundEvent
	producerDelivery    scenarioOrderedDelivery
	classificationVotes scenarioOrderedDelivery
}

type scenarioOrderedDelivery struct {
	order   []string
	next    int
	pending map[string]data.RoundEvent
}

func newScenarioNetwork(
	inboxes []chan data.RoundEvent,
	producerOrder []string,
	judgeOrder []string,
) *scenarioNetwork {
	registeredInboxes := make(map[string]chan data.RoundEvent, len(inboxes))
	for index, inbox := range inboxes {
		registeredInboxes[fmt.Sprintf("validator-%d", index+1)] = inbox
	}
	return &scenarioNetwork{
		inboxes: registeredInboxes,
		producerDelivery: scenarioOrderedDelivery{
			order: producerOrder, pending: make(map[string]data.RoundEvent),
		},
		classificationVotes: scenarioOrderedDelivery{
			// The leader handles its own classification vote directly instead of
			// sending it through the broadcaster, so delivery starts at member-1.
			order: judgeOrder, next: 1, pending: make(map[string]data.RoundEvent),
		},
	}
}

func (network *scenarioNetwork) sendToLeader(senderID, leaderID string, message *data.ConsensusMessage) error {
	if message == nil {
		return broadcast.ErrNilConsensusMessage
	}
	event := data.RoundEvent{Type: data.ConsensusMessageEvent, Message: *message}

	network.mutex.Lock()
	defer network.mutex.Unlock()

	switch message.ConsensusMessageType {
	case data.ExecutedPromptsMessage:
		return network.enqueueOrdered(senderID, leaderID, event, &network.producerDelivery)
	case data.AnswerClassificationVoteConsensusMessage:
		return network.enqueueOrdered(senderID, leaderID, event, &network.classificationVotes)
	default:
		return network.deliver(leaderID, event)
	}
}

func (network *scenarioNetwork) enqueueOrdered(
	senderID string,
	receiverID string,
	event data.RoundEvent,
	delivery *scenarioOrderedDelivery,
) error {
	if _, exists := delivery.pending[senderID]; exists {
		return fmt.Errorf("scenario network received duplicate ordered message from %s", senderID)
	}
	delivery.pending[senderID] = event

	for delivery.next < len(delivery.order) {
		nextSender := delivery.order[delivery.next]
		nextEvent, exists := delivery.pending[nextSender]
		if !exists {
			break
		}
		if err := network.deliver(receiverID, nextEvent); err != nil {
			return err
		}
		delete(delivery.pending, nextSender)
		delivery.next++
	}
	return nil
}

func (network *scenarioNetwork) broadcast(message *data.ConsensusMessage, senderID string, receivers []string) error {
	if message == nil {
		return broadcast.ErrNilConsensusMessage
	}

	network.mutex.Lock()
	defer network.mutex.Unlock()
	for _, receiverID := range receivers {
		if receiverID == senderID {
			continue
		}
		if err := network.deliver(receiverID, data.RoundEvent{
			Type: data.ConsensusMessageEvent, Message: cloneScenarioConsensusMessage(message),
		}); err != nil {
			return err
		}
	}
	return nil
}

// cloneScenarioConsensusMessage models serialization at the network boundary.
// In particular, block validation writes computed hashes into the proposed
// block header, so sharing one in-memory block across nodes creates a data race
// that would not exist when each node decodes its own network payload.
func cloneScenarioConsensusMessage(message *data.ConsensusMessage) data.ConsensusMessage {
	cloned := *message
	if message.ProposedBlockMessage == nil {
		return cloned
	}

	proposedBlock := *message.ProposedBlockMessage
	if message.ProposedBlockMessage.Block != nil {
		block := *message.ProposedBlockMessage.Block
		block.Header.BodyHash = copyBytes(block.Header.BodyHash)
		block.Header.HeaderHash = copyBytes(block.Header.HeaderHash)
		block.Header.PreviousHash = copyBytes(block.Header.PreviousHash)
		block.Header.RootHash = copyBytes(block.Header.RootHash)
		block.Header.PreviousRootHash = copyBytes(block.Header.PreviousRootHash)
		block.Body.Transactions = cloneTransactions(block.Body.Transactions)
		proposedBlock.Block = &block
	}
	cloned.ProposedBlockMessage = &proposedBlock
	return cloned
}

func (network *scenarioNetwork) deliver(receiverID string, event data.RoundEvent) error {
	inbox, exists := network.inboxes[receiverID]
	if !exists {
		return broadcast.ErrInvalidValidator
	}
	inbox <- event
	return nil
}

type scenarioBroadcaster struct {
	nodeID  string
	network *scenarioNetwork
}

func (broadcaster *scenarioBroadcaster) SendVoteToLeader(message *data.ConsensusMessage, leaderID string) error {
	return broadcaster.network.sendToLeader(broadcaster.nodeID, leaderID, message)
}

func (broadcaster *scenarioBroadcaster) SendAnswerClassificationVoteToLeader(
	message *data.ConsensusMessage,
	leaderID string,
) error {
	return broadcaster.network.sendToLeader(broadcaster.nodeID, leaderID, message)
}

func (broadcaster *scenarioBroadcaster) BroadcastProposedBlock(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *scenarioBroadcaster) BroadcastAggregatedVotes(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *scenarioBroadcaster) BroadcastAnswerEvidence(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func (broadcaster *scenarioBroadcaster) BroadcastAnswerClassificationCertificate(
	message *data.ConsensusMessage,
	myID string,
	receivers []string,
) error {
	return broadcaster.network.broadcast(message, myID, receivers)
}

func startScenarioNodes(nodes []*integrationTestNode) []<-chan struct{} {
	done := make([]<-chan struct{}, 0, len(nodes))
	for _, node := range nodes {
		nodeDone := make(chan struct{})
		done = append(done, nodeDone)
		go func(currentNode *integrationTestNode) {
			defer close(nodeDone)
			currentNode.loop.Run()
		}(node)
	}
	return done
}

func firstScenarioError(nodes []*integrationTestNode) error {
	for _, node := range nodes {
		select {
		case err := <-node.loop.Errors():
			return fmt.Errorf("%s: %w", node.id, err)
		default:
		}
	}
	return nil
}

func scenarioNodeByID(t *testing.T, nodes []*integrationTestNode, id string) *integrationTestNode {
	t.Helper()
	for _, node := range nodes {
		if node.id == id {
			return node
		}
	}
	require.FailNow(t, "scenario node not found", id)
	return nil
}

func requireScenarioFinalizedState(
	t *testing.T,
	scenario miniRoundTwoScenario,
	nodes []*integrationTestNode,
	committees scenarioCommittees,
	expectedFrequencies data.SubdomainsFrequency,
	firstMiniRoundOneBlock *data.BlockOnChain,
	firstMiniRoundTwoBlock *data.BlockOnChain,
	classificationVotes []*data.AnswerClassificationVote,
	miniRoundOneKey data.RoundKey,
	miniRoundTwoKey data.RoundKey,
) {
	t.Helper()

	require.Equal(t, expectedFrequencies, firstMiniRoundOneBlock.SubdomainsFrequencies)
	require.NotNil(t, firstMiniRoundTwoBlock.AnswerEvidence)
	require.Len(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, len(scenario.Transactions))
	require.Len(t, firstMiniRoundTwoBlock.AnswerClassifications, len(scenario.Transactions))

	expectedProducers := scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Expected.AnswerEvidenceProducers)
	sort.Strings(expectedProducers)
	require.Equal(t, expectedProducers, firstMiniRoundTwoBlock.AnswerEvidence.Signers)
	require.Len(t, classificationVotes, scenario.Network.Quorum)

	expectedVoters := scenarioIDsForRoles(t, committees.miniRoundTwo, scenario.Expected.ClassificationVoters)
	sort.Strings(expectedVoters)
	actualVoters := make([]string, 0, len(classificationVotes))
	rolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)
	for _, vote := range classificationVotes {
		actualVoters = append(actualVoters, vote.JudgeID)
		require.Len(t, vote.AnswerClassifications, scenario.Network.Quorum*len(scenario.Transactions))
		for _, answerClassification := range vote.AnswerClassifications {
			require.Equal(
				t,
				expectedScenarioCategory(
					t,
					scenario,
					rolesByValidator[vote.JudgeID],
					answerClassification.CandidateID,
					firstMiniRoundTwoBlock.AnswerEvidence,
				),
				answerClassification.Category,
			)
		}
	}
	require.Equal(t, expectedVoters, actualVoters)

	requireScenarioTransactions(
		t,
		scenario,
		firstMiniRoundTwoBlock,
		expectedProducers,
		rolesByValidator,
	)

	for _, node := range nodes {
		miniRoundOneBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
		require.NoError(t, err)
		require.Equal(t, firstMiniRoundOneBlock.Block.Header.HeaderHash, miniRoundOneBlock.Block.Header.HeaderHash)
		require.Equal(t, firstMiniRoundOneBlock.SubdomainsFrequencies, miniRoundOneBlock.SubdomainsFrequencies)

		miniRoundTwoBlock, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
		require.NoError(t, err)
		require.Equal(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, miniRoundTwoBlock.AggregatedExecutionResults)
		require.Equal(t, firstMiniRoundTwoBlock.AnswerEvidence, miniRoundTwoBlock.AnswerEvidence)
		require.Equal(t, firstMiniRoundTwoBlock.AnswerClassifications, miniRoundTwoBlock.AnswerClassifications)
	}
}

func requireScenarioTransactions(
	t *testing.T,
	scenario miniRoundTwoScenario,
	block *data.BlockOnChain,
	expectedProducers []string,
	rolesByValidator map[string]string,
) {
	t.Helper()
	expectedByTxHash := make(map[string]scenarioExpectedTransaction, len(scenario.Expected.Transactions))
	for _, expected := range scenario.Expected.Transactions {
		expectedByTxHash[expected.TxHash] = expected
	}

	for _, transaction := range block.AnswerClassifications {
		txHash := string(transaction.TxHash)
		expected, exists := expectedByTxHash[txHash]
		require.Truef(t, exists, "unexpected finalized transaction %s", txHash)
		require.Len(t, transaction.Counts, expected.Answers)
		require.Len(t, transaction.Groups.Correct, expected.Groups.Correct)
		require.Len(t, transaction.Groups.Hallucination, expected.Groups.Hallucination)
		require.Len(t, transaction.Groups.Malicious, expected.Groups.Malicious)
		require.Len(t, transaction.Groups.Wrong, expected.Groups.Wrong)
		require.Equal(t, expected.Status, transaction.Status)

		for _, counts := range transaction.Counts {
			require.Equal(t, txHash, string(counts.CandidateID.TxHash))
			require.Contains(t, expectedProducers, counts.CandidateID.ProducerID)
			require.Equal(t, expected.CountsPerAnswer.Correct, counts.Correct)
			require.Equal(t, expected.CountsPerAnswer.Hallucination, counts.Hallucination)
			require.Equal(t, expected.CountsPerAnswer.Malicious, counts.Malicious)
			require.Equal(t, expected.CountsPerAnswer.Wrong, counts.Wrong)
		}
	}

	for signerIndex, signerID := range block.AnswerEvidence.Signers {
		role := rolesByValidator[signerID]
		executor := scenarioExecutorForRole(t, scenario.Executors, role)
		for txHash, answer := range block.AnswerEvidence.Answers[signerIndex] {
			require.Equal(t, executor.Answers[txHash], answer.Answer)
		}
	}
}

func expectedScenarioCategory(
	t *testing.T,
	scenario miniRoundTwoScenario,
	judgeRole string,
	candidate data.AnswerCandidateID,
	evidence *data.AggregatedExecutionResultsMessage,
) data.AnswerCategory {
	t.Helper()
	judge := scenarioJudgeForRole(t, scenario.Judges, judgeRole)
	answer := scenarioAnswerForCandidate(t, evidence, candidate)
	for _, configured := range judge.Classifications {
		if configured.TxHash == string(candidate.TxHash) && configured.Answer == answer {
			return configured.Category
		}
	}
	return judge.DefaultCategory
}

func scenarioAnswerForCandidate(
	t *testing.T,
	evidence *data.AggregatedExecutionResultsMessage,
	candidate data.AnswerCandidateID,
) string {
	t.Helper()
	for signerIndex, signerID := range evidence.Signers {
		if signerID != candidate.ProducerID {
			continue
		}
		answer, exists := evidence.Answers[signerIndex][string(candidate.TxHash)]
		require.True(t, exists)
		return answer.Answer
	}
	require.FailNow(t, "candidate producer missing from answer evidence", candidate.ProducerID)
	return ""
}

type scenarioResult struct {
	Scenario   string                     `json:"scenario"`
	Passed     bool                       `json:"passed"`
	Network    scenarioNetworkFixture     `json:"network"`
	Committees scenarioResultCommittees   `json:"committees"`
	MiniRound1 scenarioMiniRoundOneResult `json:"miniRoundOne"`
	MiniRound2 scenarioMiniRoundTwoResult `json:"miniRoundTwo"`
}

type scenarioResultCommittees struct {
	MiniRoundOne []scenarioResultMember `json:"miniRoundOne"`
	MiniRoundTwo []scenarioResultMember `json:"miniRoundTwo"`
}

type scenarioResultMember struct {
	Role        string `json:"role"`
	ValidatorID string `json:"validatorId"`
}

type scenarioMiniRoundOneResult struct {
	FinalizedNodes       int                      `json:"finalizedNodes"`
	SubdomainFrequencies data.SubdomainsFrequency `json:"subdomainFrequencies"`
}

type scenarioMiniRoundTwoResult struct {
	FinalizedNodes int                          `json:"finalizedNodes"`
	AnswerEvidence scenarioAnswerEvidenceResult `json:"answerEvidence"`
	Votes          []scenarioVoteResult         `json:"classificationVotes"`
	Transactions   []scenarioTransactionResult  `json:"transactions"`
}

type scenarioAnswerEvidenceResult struct {
	LeaderID           string                 `json:"leaderId"`
	Producers          []scenarioResultMember `json:"producers"`
	SignaturesVerified bool                   `json:"signaturesVerified"`
}

type scenarioVoteResult struct {
	Judge               scenarioResultMember           `json:"judge"`
	ClassificationCount int                            `json:"classificationCount"`
	CategoryTotals      scenarioExpectedCategoryCounts `json:"categoryTotals"`
}

type scenarioTransactionResult struct {
	TxHash     string                       `json:"txHash"`
	Status     data.TransactionAnswerStatus `json:"status"`
	Candidates []scenarioCandidateResult    `json:"candidates"`
	Groups     scenarioGroupsResult         `json:"groups"`
}

type scenarioCandidateResult struct {
	ProducerRole string                         `json:"producerRole"`
	ProducerID   string                         `json:"producerId"`
	Answer       string                         `json:"answer"`
	Counts       scenarioExpectedCategoryCounts `json:"counts"`
}

type scenarioGroupsResult struct {
	Correct       []string `json:"correct"`
	Hallucination []string `json:"hallucination"`
	Malicious     []string `json:"malicious"`
	Wrong         []string `json:"wrong"`
}

func writeScenarioResult(
	t *testing.T,
	scenario miniRoundTwoScenario,
	committees scenarioCommittees,
	miniRoundOneBlock *data.BlockOnChain,
	miniRoundTwoBlock *data.BlockOnChain,
	votes []*data.AnswerClassificationVote,
) {
	t.Helper()

	rolesByValidator := scenarioRolesByValidator(committees.miniRoundTwo)
	result := scenarioResult{
		Scenario: scenario.Name,
		Passed:   true,
		Network:  scenario.Network,
		Committees: scenarioResultCommittees{
			MiniRoundOne: scenarioResultCommittee(committees.miniRoundOne),
			MiniRoundTwo: scenarioResultCommittee(committees.miniRoundTwo),
		},
		MiniRound1: scenarioMiniRoundOneResult{
			FinalizedNodes:       scenario.Network.RegisteredNodes,
			SubdomainFrequencies: miniRoundOneBlock.SubdomainsFrequencies,
		},
		MiniRound2: scenarioMiniRoundTwoResult{
			FinalizedNodes: scenario.Network.RegisteredNodes,
			AnswerEvidence: scenarioAnswerEvidenceResult{
				LeaderID:           miniRoundTwoBlock.AnswerEvidence.SenderID,
				Producers:          scenarioResultProducers(miniRoundTwoBlock.AnswerEvidence.Signers, rolesByValidator),
				SignaturesVerified: true,
			},
			Votes:        scenarioResultVotes(votes, rolesByValidator),
			Transactions: scenarioResultTransactions(miniRoundTwoBlock, rolesByValidator),
		},
	}

	encoded, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	encoded = append(encoded, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(scenario.directory, "result.json"), encoded, 0o644))
}

func scenarioResultCommittee(committee []string) []scenarioResultMember {
	members := make([]scenarioResultMember, 0, len(committee))
	for index, validatorID := range committee {
		members = append(members, scenarioResultMember{
			Role: scenarioCommitteeRole(index), ValidatorID: validatorID,
		})
	}
	return members
}

func scenarioResultProducers(signers []string, roles map[string]string) []scenarioResultMember {
	producers := make([]scenarioResultMember, 0, len(signers))
	for _, signerID := range signers {
		producers = append(producers, scenarioResultMember{Role: roles[signerID], ValidatorID: signerID})
	}
	return producers
}

func scenarioResultVotes(
	votes []*data.AnswerClassificationVote,
	roles map[string]string,
) []scenarioVoteResult {
	results := make([]scenarioVoteResult, 0, len(votes))
	for _, vote := range votes {
		categoryTotals := scenarioExpectedCategoryCounts{}
		for _, answerClassification := range vote.AnswerClassifications {
			switch answerClassification.Category {
			case data.AnswerCategoryCorrect:
				categoryTotals.Correct++
			case data.AnswerCategoryHallucination:
				categoryTotals.Hallucination++
			case data.AnswerCategoryMalicious:
				categoryTotals.Malicious++
			case data.AnswerCategoryWrong:
				categoryTotals.Wrong++
			}
		}
		results = append(results, scenarioVoteResult{
			Judge:               scenarioResultMember{Role: roles[vote.JudgeID], ValidatorID: vote.JudgeID},
			ClassificationCount: len(vote.AnswerClassifications),
			CategoryTotals:      categoryTotals,
		})
	}
	return results
}

func scenarioResultTransactions(
	block *data.BlockOnChain,
	roles map[string]string,
) []scenarioTransactionResult {
	answers := make(map[string]map[string]string)
	for signerIndex, signerID := range block.AnswerEvidence.Signers {
		for txHash, answer := range block.AnswerEvidence.Answers[signerIndex] {
			if answers[txHash] == nil {
				answers[txHash] = make(map[string]string)
			}
			answers[txHash][signerID] = answer.Answer
		}
	}

	transactions := make([]scenarioTransactionResult, 0, len(block.AnswerClassifications))
	for _, transaction := range block.AnswerClassifications {
		txHash := string(transaction.TxHash)
		candidates := make([]scenarioCandidateResult, 0, len(transaction.Counts))
		for _, counts := range transaction.Counts {
			producerID := counts.CandidateID.ProducerID
			candidates = append(candidates, scenarioCandidateResult{
				ProducerRole: roles[producerID],
				ProducerID:   producerID,
				Answer:       answers[txHash][producerID],
				Counts: scenarioExpectedCategoryCounts{
					Correct: counts.Correct, Hallucination: counts.Hallucination,
					Malicious: counts.Malicious, Wrong: counts.Wrong,
				},
			})
		}
		transactions = append(transactions, scenarioTransactionResult{
			TxHash:     txHash,
			Status:     transaction.Status,
			Candidates: candidates,
			Groups: scenarioGroupsResult{
				Correct:       scenarioGroupRoles(transaction.Groups.Correct, roles),
				Hallucination: scenarioGroupRoles(transaction.Groups.Hallucination, roles),
				Malicious:     scenarioGroupRoles(transaction.Groups.Malicious, roles),
				Wrong:         scenarioGroupRoles(transaction.Groups.Wrong, roles),
			},
		})
	}
	return transactions
}

func scenarioGroupRoles(candidates []data.AnswerCandidateID, roles map[string]string) []string {
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, roles[candidate.ProducerID])
	}
	return result
}

var _ broadcast.Broadcaster = (*scenarioBroadcaster)(nil)
