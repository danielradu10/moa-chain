package data

type Step uint8

const (
	StepIdle Step = iota
	StepSelectConsensusGroup
	StepAwaitProposal
	StepCollectVotes
	StepAwaitAggregatedVotes
	StepCollectExecutionResults
	StepAwaitAnswerEvidence
	StepJudgeAnswers
	StepCollectClassificationVotes
	StepAwaitClassificationCertificate
	StepFinished
	StepFailed
)
