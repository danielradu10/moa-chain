package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/testscommon"
)

func TestMiniRoundOneToMiniRoundTwo_AllNodesFinalizeSameExecutionResults(t *testing.T) {
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

	transactions, agentExecutions := loadMiniRoundTwoFullFlowFixture(t, numValidators)
	validateMiniRoundTwoFullFlowFixture(t, transactions, agentExecutions)

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
			createMiniRoundTwoAgentBackedLabeler(agentExecutions[i]),
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

	miniRoundOneKey := data.RoundKey{
		Epoch:     0,
		Round:     2,
		MiniRound: uint64(data.MiniRoundOne),
	}
	miniRoundTwoKey := data.RoundKey{
		Epoch:     miniRoundOneKey.Epoch,
		Round:     miniRoundOneKey.Round,
		MiniRound: uint64(data.MiniRoundTwo),
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type:     data.StartRoundEvent,
			RoundKey: miniRoundOneKey,
		}
	}

	require.Eventually(t, func() bool {
		select {
		case err := <-errCh:
			require.NoError(t, err)
		default:
		}

		for _, node := range nodes {
			finalizedBlock, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
			if err != nil || finalizedBlock == nil {
				return false
			}
		}

		return true
	}, 5*time.Second, 10*time.Millisecond)

	firstMiniRoundOneBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
	require.NoError(t, err)
	require.NotNil(t, firstMiniRoundOneBlock)
	require.NotEmpty(t, firstMiniRoundOneBlock.SubdomainsFrequencies)

	firstMiniRoundTwoBlock, err := nodes[0].blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
	require.NoError(t, err)
	require.NotNil(t, firstMiniRoundTwoBlock)
	require.Len(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, len(transactions))
	requireMiniRoundTwoAnswersMatchFixture(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, agentExecutions)

	for _, node := range nodes {
		finalizedMiniRoundOneBlock, err := node.blockFinalizer.GetFinalizedBlockInMROne(miniRoundOneKey)
		require.NoError(t, err)
		require.NotNil(t, finalizedMiniRoundOneBlock)
		require.Equal(t, firstMiniRoundOneBlock.Block.Header.HeaderHash, finalizedMiniRoundOneBlock.Block.Header.HeaderHash)
		require.Equal(t, firstMiniRoundOneBlock.SubdomainsFrequencies, finalizedMiniRoundOneBlock.SubdomainsFrequencies)

		finalizedMiniRoundTwoBlock, err := node.blockFinalizer.GetFinalizedBlockInMRTwo(miniRoundTwoKey)
		require.NoError(t, err)
		require.NotNil(t, finalizedMiniRoundTwoBlock)
		require.Equal(t, firstMiniRoundOneBlock.Block.Header.HeaderHash, finalizedMiniRoundTwoBlock.Block.Header.HeaderHash)
		require.Equal(t, firstMiniRoundOneBlock.SubdomainsFrequencies, finalizedMiniRoundTwoBlock.SubdomainsFrequencies)
		require.Equal(t, firstMiniRoundTwoBlock.AggregatedExecutionResults, finalizedMiniRoundTwoBlock.AggregatedExecutionResults)

		appendMiniRoundTwoAggregatedExecutionResult(
			t,
			node.id,
			miniRoundTwoKey,
			finalizedMiniRoundTwoBlock.AggregatedExecutionResults,
		)
	}

	for _, inbox := range inboxes {
		inbox <- data.RoundEvent{
			Type: data.StopEvent,
		}
	}
}

type miniRoundTwoFullFlowFixture struct {
	Transactions []miniRoundOneTransactionFixture `json:"transactions"`
	Agents       []agentExecutionsFixture         `json:"agents"`
}

type agentExecutionsFixture struct {
	Agent      string                     `json:"agent"`
	Executions []executedTransactionEntry `json:"executions"`
}

type executedTransactionEntry struct {
	TxHash string   `json:"txHash"`
	Labels []string `json:"labels"`
	Answer string   `json:"answer"`
}

type agentExecutionsByTxHash struct {
	agent           string
	labelsByTxHash  map[string][]string
	answersByTxHash map[string]string
}

func loadMiniRoundTwoFullFlowFixture(
	t *testing.T,
	numAgents int,
) ([]data.Transaction, []agentExecutionsByTxHash) {
	t.Helper()

	path := filepath.Join("testData", "miniround2", "full_flow.json")

	rawData, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture miniRoundTwoFullFlowFixture
	err = json.Unmarshal(rawData, &fixture)
	require.NoError(t, err)

	transactions := make([]data.Transaction, 0, len(fixture.Transactions))
	for _, txFixture := range fixture.Transactions {
		transactions = append(transactions, createTransactionFromFixture(txFixture))
	}

	require.Len(t, fixture.Agents, numAgents)

	agents := make([]agentExecutionsByTxHash, 0, len(fixture.Agents))
	for _, agentFixture := range fixture.Agents {
		labelsByTxHash := make(map[string][]string, len(agentFixture.Executions))
		answersByTxHash := make(map[string]string, len(agentFixture.Executions))

		for _, execution := range agentFixture.Executions {
			labelsByTxHash[execution.TxHash] = copyStringSlice(execution.Labels)
			answersByTxHash[execution.TxHash] = execution.Answer
		}

		agents = append(agents, agentExecutionsByTxHash{
			agent:           agentFixture.Agent,
			labelsByTxHash:  labelsByTxHash,
			answersByTxHash: answersByTxHash,
		})
	}

	return transactions, agents
}

func validateMiniRoundTwoFullFlowFixture(
	t *testing.T,
	transactions []data.Transaction,
	agents []agentExecutionsByTxHash,
) {
	t.Helper()

	txHashes := make([]string, 0, len(transactions))
	for _, tx := range transactions {
		txHashes = append(txHashes, string(tx.GetTxHash()))
	}

	for _, agentExecution := range agents {
		require.NotEmpty(t, agentExecution.agent)

		for _, txHash := range txHashes {
			labels, ok := agentExecution.labelsByTxHash[txHash]
			require.Truef(t, ok, "agent %s has no labels for txHash %s", agentExecution.agent, txHash)
			require.Lenf(t, labels, 6, "agent %s has invalid labels count for txHash %s", agentExecution.agent, txHash)

			seenLabels := make(map[string]struct{}, len(labels))
			for _, label := range labels {
				_, ok = possibleSubDomains[label]
				require.Truef(t, ok, "agent %s has invalid label %q for txHash %s", agentExecution.agent, label, txHash)

				_, duplicated := seenLabels[label]
				require.Falsef(t, duplicated, "agent %s has duplicated label %q for txHash %s", agentExecution.agent, label, txHash)

				seenLabels[label] = struct{}{}
			}

			answer, ok := agentExecution.answersByTxHash[txHash]
			require.Truef(t, ok, "agent %s has no answer for txHash %s", agentExecution.agent, txHash)
			require.NotEmptyf(t, answer, "agent %s has empty answer for txHash %s", agentExecution.agent, txHash)
		}
	}
}

func createMiniRoundTwoAgentBackedLabeler(agentExecution agentExecutionsByTxHash) agent.Agent {
	return &testscommon.LabelerStub{
		LabelCalled: func(tx data.Transaction) ([]string, error) {
			txHash := string(tx.GetTxHash())

			labels, ok := agentExecution.labelsByTxHash[txHash]
			if !ok {
				return nil, fmt.Errorf("agent %s has no labels for txHash %s", agentExecution.agent, txHash)
			}

			return copyStringSlice(labels), nil
		},
		AnswerCalled: func(tx data.Transaction) (string, error) {
			txHash := string(tx.GetTxHash())

			answer, ok := agentExecution.answersByTxHash[txHash]
			if !ok {
				return "", fmt.Errorf("agent %s has no answer for txHash %s", agentExecution.agent, txHash)
			}

			return answer, nil
		},
	}
}

func requireMiniRoundTwoAnswersMatchFixture(
	t *testing.T,
	aggregatedResults data.AggregatedExecutionResults,
	agents []agentExecutionsByTxHash,
) {
	t.Helper()

	require.NotEmpty(t, aggregatedResults)

	previousTxHash := ""
	for _, txResult := range aggregatedResults {
		txHash := string(txResult.TxHash)
		require.Greater(t, txHash, previousTxHash)
		previousTxHash = txHash

		require.Len(t, txResult.Answers, consensusQuorum(5))
		for _, answer := range txResult.Answers {
			require.Equal(t, txResult.TxHash, answer.TxHash)
			require.Contains(t, possibleAnswersForTxHash(agents, txHash), answer.Answer)
		}
	}
}

func possibleAnswersForTxHash(agents []agentExecutionsByTxHash, txHash string) []string {
	answers := make([]string, 0, len(agents))
	for _, agentExecution := range agents {
		answer, ok := agentExecution.answersByTxHash[txHash]
		if ok {
			answers = append(answers, answer)
		}
	}

	return answers
}
func appendMiniRoundTwoAggregatedExecutionResult(
	t *testing.T,
	nodeID string,
	roundKey data.RoundKey,
	aggregatedResults data.AggregatedExecutionResults,
) {
	t.Helper()

	result := struct {
		TestName                 string                                    `json:"testName"`
		NodeID                   string                                    `json:"nodeId"`
		Timestamp                string                                    `json:"timestamp"`
		RoundKey                 data.RoundKey                             `json:"roundKey"`
		AggregatedExecutionState []serializableAggregatedExecutionTxResult `json:"aggregatedExecutionState"`
	}{
		TestName:                 t.Name(),
		NodeID:                   nodeID,
		Timestamp:                time.Now().UTC().Format(time.RFC3339Nano),
		RoundKey:                 roundKey,
		AggregatedExecutionState: serializableAggregatedExecutionResults(aggregatedResults),
	}

	encodedResult, err := json.Marshal(result)
	require.NoError(t, err)

	outputPath := filepath.Join("testData", "miniround2", "results", "aggregated_execution_results.jsonl")
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

type serializableAggregatedExecutionTxResult struct {
	TxHash  string                                  `json:"txHash"`
	Answers []serializableAggregatedExecutionAnswer `json:"answers"`
}

type serializableAggregatedExecutionAnswer struct {
	TxHash            string `json:"txHash"`
	Answer            string `json:"answer"`
	ActualConsumption uint64 `json:"actualConsumption"`
}

func serializableAggregatedExecutionResults(
	aggregatedResults data.AggregatedExecutionResults,
) []serializableAggregatedExecutionTxResult {
	serializableResults := make([]serializableAggregatedExecutionTxResult, 0, len(aggregatedResults))

	for _, txResult := range aggregatedResults {
		answers := make([]serializableAggregatedExecutionAnswer, 0, len(txResult.Answers))
		for _, answer := range txResult.Answers {
			answers = append(answers, serializableAggregatedExecutionAnswer{
				TxHash:            string(answer.TxHash),
				Answer:            answer.Answer,
				ActualConsumption: answer.ActualConsumption,
			})
		}

		serializableResults = append(serializableResults, serializableAggregatedExecutionTxResult{
			TxHash:  string(txResult.TxHash),
			Answers: answers,
		})
	}

	return serializableResults
}
