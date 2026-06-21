package processor

import (
	"moa-chain/agent"
	"moa-chain/data"
)

type promptExecutor struct {
	labeler agent.Labeler
}

func NewPromptExecutor(labeler agent.Labeler) *promptExecutor {
	return &promptExecutor{
		labeler: labeler,
	}
}

// ExecutePromptTransaction executes the prompt of a transaction.
func (pe *promptExecutor) ExecutePromptTransaction(tx data.Transaction) (*data.TransactionResult, error) {
	answer, err := pe.labeler.Answer(tx)
	if err != nil {
		return nil, err
	}

	consumption, err := calculateNumTokensFromPrompt(answer)
	if err != nil {
		return nil, err
	}

	return &data.TransactionResult{
		TxHash:            tx.GetTxHash(),
		Answer:            answer,
		ActualConsumption: consumption,
	}, nil
}
