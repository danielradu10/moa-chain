package miniround2

import (
	"errors"
)

// ErrNotEnoughExecutionResults signals that the leader received fewer execution results than the BFT quorum requires.
var ErrNotEnoughExecutionResults = errors.New("not enough execution results")

// ErrDuplicatedAnswer signals a duplicated answer.
var ErrDuplicatedAnswer = errors.New("duplicated answer")

// ErrNilAnswers signals nil answers.
var ErrNilAnswers = errors.New("nil answers")

// ErrNilAnswerEvidence signals a missing answer-evidence certificate.
var ErrNilAnswerEvidence = errors.New("nil answer evidence")

// ErrAnswerEvidenceMismatch signals inconsistent answer-evidence certificate arrays.
var ErrAnswerEvidenceMismatch = errors.New("answer evidence mismatch")

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

// ErrNilAnswerClassificationVote signals a missing classification vote message.
var ErrNilAnswerClassificationVote = errors.New("nil answer classification vote")

// ErrAnswerClassificationVoteMismatch signals vote metadata inconsistent with the current round.
var ErrAnswerClassificationVoteMismatch = errors.New("answer classification vote mismatch")

// ErrNilAnswerClassificationCertificate signals a missing classification certificate message.
var ErrNilAnswerClassificationCertificate = errors.New("nil answer classification certificate")

// ErrAnswerClassificationCertificateMismatch signals inconsistent certificate metadata or votes.
var ErrAnswerClassificationCertificateMismatch = errors.New("answer classification certificate mismatch")

// ErrClassificationVoteEvidenceMismatch prevents signing a vote for unexpected answer evidence.
var ErrClassificationVoteEvidenceMismatch = errors.New("classification vote evidence mismatch")

// ErrClassificationVotePromptMismatch prevents signing a vote with an unexpected protocol prompt.
var ErrClassificationVotePromptMismatch = errors.New("classification vote prompt mismatch")

// ErrClassificationVoteHashMismatch signals that a vote hash does not commit to its payload.
var ErrClassificationVoteHashMismatch = errors.New("classification vote hash mismatch")

// ErrMissingClassificationCollectionContext signals that the leader's trusted local vote is unavailable.
var ErrMissingClassificationCollectionContext = errors.New("missing classification collection context")

// ErrClassificationCertificateResultMismatch signals leader-provided groups that differ from deterministic aggregation.
var ErrClassificationCertificateResultMismatch = errors.New("classification certificate result mismatch")
