package agent

import (
	"moa-chain/data"
)

// AnswerJudgeRequest separates trusted protocol instructions from untrusted
// transaction prompts and candidate answers.
type AnswerJudgeRequest struct {
	SystemPrompt string
	UserPrompt   string
}

// AnswerJudge classifies candidate answers and returns structured output.
type AnswerJudge interface {
	JudgeAnswers(request AnswerJudgeRequest) (string, error)
}

// Agent defines what an agent should do
type Agent interface {
	AnswerJudge
	Label(tx data.Transaction) ([]string, error)
	Answer(tx data.Transaction) (string, error)
}
