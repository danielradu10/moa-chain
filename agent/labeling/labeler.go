package labeling

import (
	"moa-chain/agent"
	"moa-chain/data"
)

// label defines the data structure used to label our transaction
type label struct {
}

// Label finds the labels (the subdomains) of a given transaction
func (l *label) Label(tx data.Transaction) ([]string, error) {
	return nil, agent.ErrNotImplemented
}
