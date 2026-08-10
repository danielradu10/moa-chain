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

// SynthesisRequest contains the inputs for one /synthesize call: the original
// transaction and the correct answer texts recovered from the MR2 evidence.
type SynthesisRequest struct {
	Tx             data.Transaction
	CorrectAnswers []string
}

// SynthesisResult holds the synthesized answer produced for one transaction.
type SynthesisResult struct {
	TxHash []byte
	Answer string
}

// EvaluateSynthesisRequest contains the inputs for one /evaluate-synthesis
// call: the original transaction, the MR2 correct answers as reference, and
// the leader's proposed synthesized answer to evaluate.
type EvaluateSynthesisRequest struct {
	Tx                data.Transaction
	CorrectAnswers    []string
	ProposedSynthesis string
}

// EvaluateSynthesisResult holds the binary evaluation outcome for one
// transaction. Approved is true when the proposed synthesis is judged to
// faithfully combine the correct answers without introducing errors or
// injected content.
type EvaluateSynthesisResult struct {
	TxHash   []byte
	Approved bool
}

// BatchAgent is the interface used by the block executor to call the Python
// agent service. It sends all transactions in a single HTTP request per
// operation instead of one call per transaction, matching the Python service's
// batch endpoints (/label, /answer, /judge, /synthesize, /evaluate-synthesis).
type BatchAgent interface {
	AnswersJudge
	LabelBatch(txs []data.Transaction) ([]LabelResult, error)
	AnswerBatch(txs []data.Transaction) ([]AnswerResult, error)
	SynthesizeBatch(requests []SynthesisRequest) ([]SynthesisResult, error)
	EvaluateSynthesisBatch(requests []EvaluateSynthesisRequest) ([]EvaluateSynthesisResult, error)
}
