package labeling

import (
	"moa-chain/agent"
	"moa-chain/data"
)

type labeler struct{}

func (l *labeler) LabelBatch(_ []data.Transaction) ([]agent.LabelResult, error) {
	return nil, agent.ErrNotImplemented
}

func (l *labeler) AnswerBatch(_ []data.Transaction) ([]agent.AnswerResult, error) {
	return nil, agent.ErrNotImplemented
}

func (l *labeler) JudgeTransactionAnswers(_ agent.AnswerJudgeRequest) (string, error) {
	return "", agent.ErrNotImplemented
}
