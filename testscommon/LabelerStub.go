package testscommon

import (
	"moa-chain/agent"
	"moa-chain/data"
)

type LabelerStub struct {
	Err                error
	AnswerErr          error
	LabelsByTxHash     map[string][]string
	AnswersByTxHash    map[string]string
	LabelCalled        func(tx data.Transaction) ([]string, error)
	AnswerCalled       func(tx data.Transaction) (string, error)
	JudgeAnswersCalled func(request agent.AnswerJudgeRequest) (string, error)
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
