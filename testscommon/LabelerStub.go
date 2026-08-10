package testscommon

import (
	"moa-chain/agent"
	"moa-chain/data"
)

// LabelerStub implements agent.BatchAgent.
// Tests configure responses via the *Called hook functions.
type LabelerStub struct {
	JudgeAnswersCalled           func(request agent.AnswerJudgeRequest) (string, error)
	LabelBatchCalled             func(txs []data.Transaction) ([]agent.LabelResult, error)
	AnswerBatchCalled            func(txs []data.Transaction) ([]agent.AnswerResult, error)
	SynthesizeBatchCalled        func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error)
	EvaluateSynthesisBatchCalled func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error)
}

func (tl *LabelerStub) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	if tl.JudgeAnswersCalled != nil {
		return tl.JudgeAnswersCalled(request)
	}

	return "", agent.ErrNotImplemented
}

func (tl *LabelerStub) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	if tl.LabelBatchCalled != nil {
		return tl.LabelBatchCalled(txs)
	}

	results := make([]agent.LabelResult, len(txs))
	for i, tx := range txs {
		results[i] = agent.LabelResult{TxHash: tx.GetTxHash(), Labels: []string{}}
	}
	return results, nil
}

func (tl *LabelerStub) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	if tl.AnswerBatchCalled != nil {
		return tl.AnswerBatchCalled(txs)
	}

	results := make([]agent.AnswerResult, len(txs))
	for i, tx := range txs {
		results[i] = agent.AnswerResult{TxHash: tx.GetTxHash(), Answer: ""}
	}
	return results, nil
}

func (tl *LabelerStub) SynthesizeBatch(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
	if tl.SynthesizeBatchCalled != nil {
		return tl.SynthesizeBatchCalled(requests)
	}

	return nil, agent.ErrNotImplemented
}

func (tl *LabelerStub) EvaluateSynthesisBatch(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
	if tl.EvaluateSynthesisBatchCalled != nil {
		return tl.EvaluateSynthesisBatchCalled(requests)
	}

	return nil, agent.ErrNotImplemented
}
