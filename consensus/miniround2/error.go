package miniround2

import (
	"errors"
)

// ErrDuplicatedAnswer signals a duplicated answer.
var ErrDuplicatedAnswer = errors.New("duplicated answer")

// ErrNilAnswers signals nil answers.
var ErrNilAnswers = errors.New("nil answers")

// ErrNilAggregatedExecutionResults signals a nil aggregated execution results message.
var ErrNilAggregatedExecutionResults = errors.New("nil aggregated execution results")

// ErrAggregatedExecutionResultsMismatch signals inconsistent aggregated execution results arrays.
var ErrAggregatedExecutionResultsMismatch = errors.New("aggregated execution results mismatch")

// ErrMessageNotFromLeader signals that the message was not sent by the expected leader.
var ErrMessageNotFromLeader = errors.New("message not from leader")

// ErrOnlyLeaderCanCollectVotes signals that a non-leader received a message in the collecting step.
var ErrOnlyLeaderCanCollectVotes = errors.New("only leader can collect votes")

// ErrSignerIsNotValidator signals that the signer is not a validator.
var ErrSignerIsNotValidator = errors.New("signer is not validator")

// ErrValidatorNotPartOfConsensusGroup signals that the validator is not part of the consensus group.
var ErrValidatorNotPartOfConsensusGroup = errors.New("validator not part of consensus group")

// ErrCanonicalBlockHashMismatch signals that the answers refer to a different canonical block.
var ErrCanonicalBlockHashMismatch = errors.New("canonical block hash mismatch")

// ErrExecutedPromptsAnswersMismatch signals that the answers do not match the canonical block transactions.
var ErrExecutedPromptsAnswersMismatch = errors.New("executed prompts answers mismatch")

// ErrExecutionResultHashMismatch signals that the answers hash does not match the signed block hash.
var ErrExecutionResultHashMismatch = errors.New("execution result hash mismatch")

// ErrInvalidAnswerCandidate signals a candidate with missing or malformed identity fields.
var ErrInvalidAnswerCandidate = errors.New("invalid answer candidate")

// ErrDuplicatedAnswerCandidate signals that a candidate appears more than once.
var ErrDuplicatedAnswerCandidate = errors.New("duplicated answer candidate")

// ErrMissingAnswerCandidate signals that a classification does not cover an expected candidate.
var ErrMissingAnswerCandidate = errors.New("missing answer candidate")

// ErrUnknownAnswerCandidate signals that a classification contains an unexpected candidate.
var ErrUnknownAnswerCandidate = errors.New("unknown answer candidate")

// ErrInvalidAnswerCategory signals a category not defined by the protocol.
var ErrInvalidAnswerCategory = errors.New("invalid answer category")

// ErrNonCanonicalClassification signals protocol data that is not canonically ordered.
var ErrNonCanonicalClassification = errors.New("non-canonical answer classification")

// ErrDuplicatedClassificationJudge signals more than one vote from the same judge.
var ErrDuplicatedClassificationJudge = errors.New("duplicated classification judge")

// ErrInvalidClassificationVote signals a malformed classification vote.
var ErrInvalidClassificationVote = errors.New("invalid classification vote")
