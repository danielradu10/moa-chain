package miniround2

import (
	"moa-chain/data"
)

// MiniRoundTwoHandler defines what a handler for the second mini-round should do.
type MiniRoundTwoHandler interface {
	HandleConsensusSelection(key data.RoundKey) (string, error)
	HandleBlockExecution(roundKey data.RoundKey) error
	HandleExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error
	HandleExecutedPromptsCollectionTimeout(roundKey data.RoundKey) error
	HandleAnswerEvidence(roundKey data.RoundKey, message *data.AggregatedExecutionResultsMessage) error
	HandleAnswerClassificationVote(roundKey data.RoundKey, vote *data.AnswerClassificationVote) error
	HandleClassificationGracePeriodElapsed(roundKey data.RoundKey) error
	HandleAnswerClassificationCertificate(roundKey data.RoundKey, certificate *data.AnswerClassificationCertificate) error
	HasVerifiedAnswerEvidence(roundKey data.RoundKey) bool
	// WaitForPendingWork blocks until all in-flight judge goroutines have
	// finished. Call after the event loop has stopped to ensure a clean shutdown.
	WaitForPendingWork()
}
