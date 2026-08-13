package integrationtests

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/testscommon"
)

var errMR3EvaluatorExecution = errors.New("mr3 evaluator execution failed")

func createMR3ScenarioAgent(
	t *testing.T,
	scenario miniRoundThreeScenario,
	mr2Role string,
	mr3Role string,
) agent.BatchAgent {
	t.Helper()

	labelsByTxHash := make(map[string][]string, len(scenario.Transactions))
	for _, transaction := range scenario.Transactions {
		labelsByTxHash[transaction.TxHash] = copyStringSlice(transaction.Labels)
	}

	executor := scenarioExecutorForRole(t, scenario.Executors, mr2Role)
	judge := scenarioJudgeForRole(t, scenario.Judges, mr2Role)
	evaluator := mr3EvaluatorForRole(t, scenario.Evaluators, mr3Role)
	answers := executor.Answers
	synthesizedAnswers := scenario.Synthesizer.Answers

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
		SynthesizeBatchCalled: func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
			results := make([]agent.SynthesisResult, 0, len(requests))
			for _, req := range requests {
				txHash := string(req.Tx.GetTxHash())
				answer, ok := synthesizedAnswers[txHash]
				require.Truef(t, ok, "synthesizer has no answer for txHash %s", txHash)
				results = append(results, agent.SynthesisResult{TxHash: req.Tx.GetTxHash(), Answer: answer})
			}
			return results, nil
		},
		EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
			return executeMR3Evaluator(requests, evaluator)
		},
	}
}

func mr3EvaluatorForRole(
	t *testing.T,
	profiles []scenarioEvaluatorFixture,
	role string,
) scenarioEvaluatorFixture {
	t.Helper()

	defaultProfile := scenarioEvaluatorFixture{}
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

func executeMR3Evaluator(
	requests []agent.EvaluateSynthesisRequest,
	evaluator scenarioEvaluatorFixture,
) ([]agent.EvaluateSynthesisResult, error) {
	switch evaluator.Mode {
	case "approve_all":
		results := make([]agent.EvaluateSynthesisResult, len(requests))
		for i, req := range requests {
			results[i] = agent.EvaluateSynthesisResult{TxHash: req.Tx.GetTxHash(), Approved: true}
		}
		return results, nil
	case "reject_all":
		results := make([]agent.EvaluateSynthesisResult, len(requests))
		for i, req := range requests {
			results[i] = agent.EvaluateSynthesisResult{TxHash: req.Tx.GetTxHash(), Approved: false}
		}
		return results, nil
	case "execution_error":
		return nil, errMR3EvaluatorExecution
	default:
		return nil, errMR3EvaluatorExecution
	}
}
