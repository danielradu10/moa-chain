package agent

import (
	"moa-chain/data"
)

// Labeler defines what a labeler should do
type Labeler interface {
	Label(tx data.Transaction, amILeader bool) ([]string, error)
}
