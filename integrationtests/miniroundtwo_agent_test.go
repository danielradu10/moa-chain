package integrationtests

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/testscommon"
)

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
	input, err := decodeScenarioJudgeInput(request)
	if err != nil {
		return "", err
	}

	txHashBytes, err := hex.DecodeString(input.TransactionHash)
	if err != nil {
		return "", err
	}

	output := scenarioJudgeOutput{Classifications: make([]scenarioJudgeClassification, 0, len(input.Candidates))}
	for _, candidate := range input.Candidates {
		output.Classifications = append(output.Classifications, scenarioJudgeClassification{
			CandidateID: candidate.CandidateID,
			Category:    scenarioCategoryForCandidate(string(txHashBytes), candidate.Answer, judge),
		})
	}

	encoded, err := json.Marshal(output)
	return string(encoded), err
}

type scenarioJudgeInput struct {
	TransactionHash string                   `json:"transactionHash"`
	Candidates      []scenarioJudgeCandidate `json:"candidates"`
}

type scenarioJudgeCandidate struct {
	CandidateID string `json:"candidateId"`
	Answer      string `json:"answer"`
}

type scenarioJudgeOutput struct {
	Classifications []scenarioJudgeClassification `json:"classifications"`
}

type scenarioJudgeClassification struct {
	CandidateID string              `json:"candidateId"`
	Category    data.AnswerCategory `json:"category"`
}

func decodeScenarioJudgeInput(request agent.AnswerJudgeRequest) (scenarioJudgeInput, error) {
	var input scenarioJudgeInput
	err := json.Unmarshal([]byte(request.UserPrompt), &input)
	return input, err
}

func scenarioCategoryForCandidate(
	txHash string,
	answer string,
	judge scenarioJudgeFixture,
) data.AnswerCategory {
	for _, configured := range judge.Classifications {
		if configured.TxHash == txHash && configured.Answer == answer {
			return configured.Category
		}
	}
	return judge.DefaultCategory
}
