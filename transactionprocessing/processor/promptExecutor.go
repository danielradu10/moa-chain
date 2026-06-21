package processor

import (
	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/transactionprocessing"
)

type promptExecutor struct {
	agent agent.Agent
}

// NewPromptExecutor creates a new prompt executor.
func NewPromptExecutor(agent agent.Agent) (*promptExecutor, error) {
	if agent == nil {
		return nil, transactionprocessing.ErrNilAgent
	}
	return &promptExecutor{
		agent: agent,
	}, nil
}

// ExecutePromptTransaction executes the prompt of a transaction.
// TODO maybe return more metadata per transaction.
func (pe *promptExecutor) ExecutePromptTransaction(tx data.Transaction) (*data.TransactionResult, error) {
	answer, err := pe.agent.Answer(tx)
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
