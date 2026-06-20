package agent

import (
	"moa-chain/data"
)

// Labeler defines what a labeler should do
type Labeler interface {
	Label(tx data.Transaction) ([]string, error)
	Answer(tx data.Transaction) (string, error)
}
