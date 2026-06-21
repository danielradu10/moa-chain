package miniround2

import (
	"errors"
)

// ErrDuplicatedAnswer signals a duplicated answer.
var ErrDuplicatedAnswer = errors.New("duplicated answer")

// ErrNilAnswers signals nil answers.
var ErrNilAnswers = errors.New("nil answers")

// ErrOnlyLeaderCanCollectVotes signals that a non-leader received a message in the collecting step.
var ErrOnlyLeaderCanCollectVotes = errors.New("only leader can collect votes")

// ErrSignerIsNotValidator signals that the signer is not a validator.
var ErrSignerIsNotValidator = errors.New("signer is not validator")

// ErrValidatorNotPartOfConsensusGroup signals that the validator is not part of the consensus group.
var ErrValidatorNotPartOfConsensusGroup = errors.New("validator not part of consensus group")
