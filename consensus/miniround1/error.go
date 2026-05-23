package miniround1

import (
	"errors"
)

// ErrNilProposedBlockMessage signals a nil proposed block message
var ErrNilProposedBlockMessage = errors.New("nil proposed block message")

// ErrSignerIsNotValidator signals that the signer is not a validator
var ErrSignerIsNotValidator = errors.New("signer is not validator")

// ErrValidatorNotPartOfConsensusGroup signals that the validator is not part of the consensus group
var ErrValidatorNotPartOfConsensusGroup = errors.New("validator not part of consensus group")

// ErrNilBlock signals a nil block.
var ErrNilBlock = errors.New("nil block")

// ErrNilVote signals a nil block.
var ErrNilVote = errors.New("nil vote")

var ErrMessageNotFromLeader = errors.New("message not from leader")

var ErrOnlyLeaderCanCollectVotes = errors.New("only leader can collect votes")
