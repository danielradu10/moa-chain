package miniround2

import (
	"moa-chain/data"
)

// MiniRoundTwoHandler defines what a handler for the second mini-round should do.
type MiniRoundTwoHandler interface {
	HandleConsensusSelection(key data.RoundKey) (string, error)
	HandleBlockExecution(roundKey data.RoundKey) error
	HandleExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error
	HandleAggregatedExecutionResults(roundKey data.RoundKey, message *data.AggregatedExecutionResultsMessage) error
}
