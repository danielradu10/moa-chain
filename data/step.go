package data

type Step uint8

const (
	StepIdle Step = iota
	StepSelectConsensusGroup
	StepAwaitProposal
	StepCollectVotes
	StepAwaitAggregatedVotes
	StepCollectExecutionResults
	StepAwaitAggregatedExecutionResults
	StepFinished
	StepFailed
)
