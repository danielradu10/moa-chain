package classification

import "errors"

// Candidate and assignment validation errors.
var (
	ErrInvalidAnswerCandidate     = errors.New("invalid answer candidate")
	ErrDuplicatedAnswerCandidate  = errors.New("duplicated answer candidate")
	ErrMissingAnswerCandidate     = errors.New("missing answer candidate")
	ErrUnknownAnswerCandidate     = errors.New("unknown answer candidate")
	ErrInvalidAnswerCategory      = errors.New("invalid answer category")
	ErrNonCanonicalClassification = errors.New("non-canonical answer classification")
)

// Vote-set validation and aggregation errors.
var (
	ErrDuplicatedClassificationJudge      = errors.New("duplicated classification judge")
	ErrInvalidClassificationVote          = errors.New("invalid classification vote")
	ErrInvalidClassificationCommitteeSize = errors.New("invalid classification committee size")
	ErrInvalidClassificationVoteCount     = errors.New("invalid classification vote count")
	ErrClassificationVoteContextMismatch  = errors.New("classification vote context mismatch")
)

// Local judge request and response errors. A judge failure invalidates the
// node's entire vote; callers must never publish partial classifications.
var (
	ErrNilAnswerJudge             = errors.New("nil answer judge")
	ErrInvalidAnswerJudgeInput    = errors.New("invalid answer judge input")
	ErrAnswerJudgeExecutionFailed = errors.New("answer judge execution failed")
	ErrInvalidAnswerJudgeResponse = errors.New("invalid answer judge response")
)
