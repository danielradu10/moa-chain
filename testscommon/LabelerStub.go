package testscommon

import (
	"moa-chain/agent"
	"moa-chain/data"
)

// LabelerStub implements both agent.Agent and agent.BatchAgent.
// Tests configure responses via the map fields or the *Called hook functions.
type LabelerStub struct {
	Err                error
	AnswerErr          error
	LabelsByTxHash     map[string][]string
	AnswersByTxHash    map[string]string
	LabelCalled        func(tx data.Transaction) ([]string, error)
	AnswerCalled       func(tx data.Transaction) (string, error)
	JudgeAnswersCalled func(request agent.AnswerJudgeRequest) (string, error)
	LabelBatchCalled   func(txs []data.Transaction) ([]agent.LabelResult, error)
	AnswerBatchCalled  func(txs []data.Transaction) ([]agent.AnswerResult, error)
}

func (tl *LabelerStub) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	if tl.JudgeAnswersCalled != nil {
		return tl.JudgeAnswersCalled(request)
	}

	return "", agent.ErrNotImplemented
}

func (tl *LabelerStub) Label(tx data.Transaction) ([]string, error) {
	if tl.LabelCalled != nil {
		return tl.LabelCalled(tx)
	}

	if tl.Err != nil {
		return nil, tl.Err
	}

	labels, ok := tl.LabelsByTxHash[string(tx.GetTxHash())]
	if !ok {
		return []string{}, nil
	}

	return labels, nil
}

func (tl *LabelerStub) Answer(tx data.Transaction) (string, error) {
	if tl.AnswerCalled != nil {
		return tl.AnswerCalled(tx)
	}

	if tl.AnswerErr != nil {
		return "", tl.AnswerErr
	}

	answer, ok := tl.AnswersByTxHash[string(tx.GetTxHash())]
	if !ok {
		return "", nil
	}

	return answer, nil
}

// LabelBatch implements agent.BatchAgent. Delegates to LabelBatchCalled if set,
// otherwise falls back to calling Label per transaction (uses existing map/hook).
func (tl *LabelerStub) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	if tl.LabelBatchCalled != nil {
		return tl.LabelBatchCalled(txs)
	}

	results := make([]agent.LabelResult, 0, len(txs))
	for _, tx := range txs {
		labels, err := tl.Label(tx)
		if err != nil {
			return nil, err
		}
		results = append(results, agent.LabelResult{
			TxHash: tx.GetTxHash(),
			Labels: labels,
		})
	}
	return results, nil
}

// AnswerBatch implements agent.BatchAgent. Delegates to AnswerBatchCalled if set,
// otherwise falls back to calling Answer per transaction (uses existing map/hook).
func (tl *LabelerStub) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	if tl.AnswerBatchCalled != nil {
		return tl.AnswerBatchCalled(txs)
	}

	results := make([]agent.AnswerResult, 0, len(txs))
	for _, tx := range txs {
		answer, err := tl.Answer(tx)
		if err != nil {
			return nil, err
		}
		results = append(results, agent.AnswerResult{
			TxHash: tx.GetTxHash(),
			Answer: answer,
		})
	}
	return results, nil
}
