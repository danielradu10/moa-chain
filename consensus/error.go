package consensus

import (
	"errors"
)

var ErrNilConsensusMessage = errors.New("consensus message is nil")

var ErrUnknownConsensusMessage = errors.New("unknown consensus message")

var ErrUnexpectedMessageForStep = errors.New("unexpected consensus message")

var ErrNilProposedBlockMessage = errors.New("proposed block is nil")

var ErrMessageForDifferentRound = errors.New("different round")

var ErrNilVote = errors.New("vote is nil")

var ErrNilAggregatedVotes = errors.New("aggregated votes is nil")

var ErrNilExecutedPrompts = errors.New("executed prompts is nil")

var ErrNilAggregatedExecutionResults = errors.New("aggregated execution results is nil")

var ErrNilAnswerClassificationVote = errors.New("answer classification vote is nil")

var ErrNilAnswerClassificationCertificate = errors.New("answer classification certificate is nil")

var ErrStaleTimeout = errors.New("stale timeout")

var ErrProposalTimeout = errors.New("proposal timeout")

var ErrNotEnoughVotes = errors.New("not enough votes")

var ErrAggregatedVotesTimeout = errors.New("aggregated votes timeout")

var ErrNotEnoughExecutionResults = errors.New("not enough execution results")

var ErrAggregatedExecutionResultsTimeout = errors.New("aggregated execution results timeout")

var ErrAnswerEvidenceTimeout = errors.New("answer evidence timeout")

var ErrAnswerJudgingTimeout = errors.New("answer judging timeout")

var ErrNotEnoughAnswerClassificationVotes = errors.New("not enough answer classification votes")

var ErrAnswerClassificationCertificateTimeout = errors.New("answer classification certificate timeout")
