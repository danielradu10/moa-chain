package labeling

import (
	agent2 "moa-chain/agent"
	"moa-chain/data"
)

// agent defines the data structure used to agent our transaction
type agent struct {
}

// Label finds the labels (the subdomains) of a given transaction
func (l *agent) Label(tx data.Transaction) ([]string, error) {
	return nil, agent2.ErrNotImplemented
}

func (l *agent) Answer(tx data.Transaction) (string, error) {
	return "", agent2.ErrNotImplemented
}
