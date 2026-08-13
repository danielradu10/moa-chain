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

	// Mini-Round Three steps
	StepSynthesizeAnswers             // leader synthesis goroutine is running
	StepCollectSynthesisVotes         // leader collecting binary approval votes
	StepAwaitProposedSynthesis        // non-leader waiting for leader's proposal
	StepAwaitAggregatedSynthesisVotes // non-leader waiting for aggregated votes

	StepFinished
	StepFailed
)
