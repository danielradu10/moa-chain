package integrationtests

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/testscommon"
)

var errScenarioJudgeExecution = errors.New("scenario judge execution failed")

func createScenarioAgent(t *testing.T, scenario miniRoundTwoScenario, role string) agent.BatchAgent {
	t.Helper()

	labelsByTxHash := make(map[string][]string, len(scenario.Transactions))
	for _, transaction := range scenario.Transactions {
		labelsByTxHash[transaction.TxHash] = copyStringSlice(transaction.Labels)
	}

	executor := scenarioExecutorForRole(t, scenario.Executors, role)
	judge := scenarioJudgeForRole(t, scenario.Judges, role)
	answers := executor.Answers

	return &testscommon.LabelerStub{
		LabelBatchCalled: func(txs []data.Transaction) ([]agent.LabelResult, error) {
			results := make([]agent.LabelResult, 0, len(txs))
			for _, tx := range txs {
				labels := copyStringSlice(labelsByTxHash[string(tx.GetTxHash())])
				results = append(results, agent.LabelResult{TxHash: tx.GetTxHash(), Labels: labels})
			}
			return results, nil
		},
		AnswerBatchCalled: func(txs []data.Transaction) ([]agent.AnswerResult, error) {
			results := make([]agent.AnswerResult, 0, len(txs))
			for _, tx := range txs {
				results = append(results, agent.AnswerResult{
					TxHash: tx.GetTxHash(),
					Answer: answers[string(tx.GetTxHash())],
				})
			}
			return results, nil
		},
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
	txHash := string(txHashBytes)

	if judge.Mode == "execution_error" && judge.ErrorTxHash == txHash {
		return "", errScenarioJudgeExecution
	}
	if judge.ErrorTxHash == txHash {
		if response, handled := malformedScenarioJudgeResponse(input, judge.Mode); handled {
			return response, nil
		}
	}

	output := scenarioJudgeOutput{Classifications: make([]scenarioJudgeClassification, 0, len(input.Candidates))}
	for _, candidate := range input.Candidates {
		output.Classifications = append(output.Classifications, scenarioJudgeClassification{
			CandidateID: candidate.CandidateID,
			Category:    scenarioCategoryForCandidate(txHash, candidate.Answer, judge),
		})
	}

	encoded, err := json.Marshal(output)
	return string(encoded), err
}

func malformedScenarioJudgeResponse(input scenarioJudgeInput, mode string) (string, bool) {
	switch mode {
	case "malformed_json":
		return `{"classifications":[`, true
	case "missing_classification":
		output := scenarioJudgeOutput{Classifications: make([]scenarioJudgeClassification, 0, len(input.Candidates)-1)}
		for index := 0; index < len(input.Candidates)-1; index++ {
			output.Classifications = append(output.Classifications, scenarioJudgeClassification{
				CandidateID: input.Candidates[index].CandidateID,
				Category:    data.AnswerCategoryCorrect,
			})
		}
		encoded, err := json.Marshal(output)
		return string(encoded), err == nil
	case "unknown_answer":
		output := scenarioJudgeOutput{Classifications: make([]scenarioJudgeClassification, 0, len(input.Candidates)+1)}
		for _, candidate := range input.Candidates {
			output.Classifications = append(output.Classifications, scenarioJudgeClassification{
				CandidateID: candidate.CandidateID,
				Category:    data.AnswerCategoryCorrect,
			})
		}
		output.Classifications = append(output.Classifications, scenarioJudgeClassification{
			CandidateID: "unknown-candidate",
			Category:    data.AnswerCategoryCorrect,
		})
		encoded, err := json.Marshal(output)
		return string(encoded), err == nil
	case "invalid_category":
		output := scenarioJudgeOutput{Classifications: make([]scenarioJudgeClassification, 0, len(input.Candidates))}
		for _, candidate := range input.Candidates {
			output.Classifications = append(output.Classifications, scenarioJudgeClassification{
				CandidateID: candidate.CandidateID,
				Category:    data.AnswerCategory("INVALID_CATEGORY"),
			})
		}
		encoded, err := json.Marshal(output)
		return string(encoded), err == nil
	default:
		return "", false
	}
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
