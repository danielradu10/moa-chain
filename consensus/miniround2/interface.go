package miniround2

import (
	"moa-chain/data"
)

// MiniRoundTwoHandler defines what a handler for the second mini-round should do.
type MiniRoundTwoHandler interface {
	HandleConsensusSelection(key data.RoundKey) (string, error)
	HandleBlockExecution(roundKey data.RoundKey) error
	HandleExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error
	HandleAnswerEvidence(roundKey data.RoundKey, message *data.AggregatedExecutionResultsMessage) error
	HandleAnswerClassificationVote(roundKey data.RoundKey, vote *data.AnswerClassificationVote) error
	HandleAnswerClassificationCertificate(roundKey data.RoundKey, certificate *data.AnswerClassificationCertificate) error
	HasVerifiedAnswerEvidence(roundKey data.RoundKey) bool
}
