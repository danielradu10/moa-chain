package agent

import (
	"moa-chain/data"
)

// Agent defines what an agent should do
type Agent interface {
	Label(tx data.Transaction) ([]string, error)
	Answer(tx data.Transaction) (string, error)
}
