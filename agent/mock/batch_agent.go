package mock

import (
	"encoding/json"
	"fmt"

	"moa-chain/agent"
	"moa-chain/data"
)

// BatchAgent returns instant deterministic results for all agent operations.
// Suitable for local development and integration testing where real LLM calls
// are not needed.
type BatchAgent struct{}

func (a *BatchAgent) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	results := make([]agent.LabelResult, len(txs))
	for i, tx := range txs {
		results[i] = agent.LabelResult{
			TxHash: tx.GetTxHash(),
			Labels: []string{"back_end_with_apis", "systems_programming"},
		}
	}
	return results, nil
}

func (a *BatchAgent) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	results := make([]agent.AnswerResult, len(txs))
	for i, tx := range txs {
		results[i] = agent.AnswerResult{
			TxHash: tx.GetTxHash(),
			Answer: fmt.Sprintf("mock answer for %x", tx.GetTxHash()),
		}
	}
	return results, nil
}

func (a *BatchAgent) SynthesizeBatch(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
	results := make([]agent.SynthesisResult, len(requests))
	for i, req := range requests {
		results[i] = agent.SynthesisResult{
			TxHash: req.Tx.GetTxHash(),
			Answer: fmt.Sprintf("mock synthesized answer for %x", req.Tx.GetTxHash()),
		}
	}
	return results, nil
}

func (a *BatchAgent) EvaluateSynthesisBatch(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
	results := make([]agent.EvaluateSynthesisResult, len(requests))
	for i, req := range requests {
		results[i] = agent.EvaluateSynthesisResult{
			TxHash:   req.Tx.GetTxHash(),
			Approved: true,
		}
	}
	return results, nil
}

// JudgeTransactionAnswers marks every candidate as Correct. The UserPrompt is
// JSON-encoded with a "candidates" array; this mock parses it and echoes every
// candidateId back with category "Correct".
func (a *BatchAgent) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	var input struct {
		Candidates []struct {
			CandidateID string `json:"candidateId"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(request.UserPrompt), &input); err != nil {
		return "", err
	}

	type classification struct {
		CandidateID string `json:"candidateId"`
		Category    string `json:"category"`
	}
	output := struct {
		Classifications []classification `json:"classifications"`
	}{Classifications: make([]classification, len(input.Candidates))}
	for i, c := range input.Candidates {
		output.Classifications[i] = classification{
			CandidateID: c.CandidateID,
			Category:    string(data.AnswerCategoryCorrect),
		}
	}
	encoded, err := json.Marshal(output)
	return string(encoded), err
}
