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

// ErrInvalidClassificationCommitteeSize signals a zero committee size.
var ErrInvalidClassificationCommitteeSize = errors.New("invalid classification committee size")

// ErrInvalidClassificationVoteCount signals a vote count outside the quorum and committee bounds.
var ErrInvalidClassificationVoteCount = errors.New("invalid classification vote count")

// ErrClassificationVoteContextMismatch signals votes for different rounds, evidence, or prompts.
var ErrClassificationVoteContextMismatch = errors.New("classification vote context mismatch")

// ErrNilAnswerJudge signals a missing answer judge.
var ErrNilAnswerJudge = errors.New("nil answer judge")

// ErrInvalidAnswerJudgeInput signals malformed answer evidence or transaction input.
var ErrInvalidAnswerJudgeInput = errors.New("invalid answer judge input")

// ErrAnswerJudgeExecutionFailed signals a local agent failure while classifying a transaction.
var ErrAnswerJudgeExecutionFailed = errors.New("answer judge execution failed")

// ErrInvalidAnswerJudgeResponse signals malformed structured judge output.
var ErrInvalidAnswerJudgeResponse = errors.New("invalid answer judge response")

// ErrNilAnswerClassificationVote signals a missing classification vote message.
var ErrNilAnswerClassificationVote = errors.New("nil answer classification vote")

// ErrAnswerClassificationVoteMismatch signals vote metadata inconsistent with the current round.
var ErrAnswerClassificationVoteMismatch = errors.New("answer classification vote mismatch")

// ErrNilAnswerClassificationCertificate signals a missing classification certificate message.
var ErrNilAnswerClassificationCertificate = errors.New("nil answer classification certificate")

// ErrAnswerClassificationCertificateMismatch signals inconsistent certificate metadata or votes.
var ErrAnswerClassificationCertificateMismatch = errors.New("answer classification certificate mismatch")
