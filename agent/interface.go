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

// AnswersJudge classifies candidate answers and returns structured output.
type AnswersJudge interface {
	JudgeTransactionAnswers(request AnswerJudgeRequest) (string, error)
}

// Agent defines what an agent should do
type Agent interface {
	AnswersJudge
	Label(tx data.Transaction) ([]string, error)
	Answer(tx data.Transaction) (string, error)
}

// LabelResult holds the labeling output for a single transaction.
type LabelResult struct {
	TxHash []byte
	Labels []string
}

// AnswerResult holds the answer output for a single transaction.
type AnswerResult struct {
	TxHash []byte
	Answer string
}

// BatchAgent is the interface used by the block executor to call the Python
// agent service. It sends all transactions in a single HTTP request per
// operation instead of one call per transaction, matching the Python service's
// batch endpoints (/label, /answer, /judge).
type BatchAgent interface {
	AnswersJudge
	LabelBatch(txs []data.Transaction) ([]LabelResult, error)
	AnswerBatch(txs []data.Transaction) ([]AnswerResult, error)
}
