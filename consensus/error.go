package consensus

import (
	"errors"
)

// ErrNoProposedBlockForRoundKey signals that no Block was cached for a specific RoundKey.
var ErrNoProposedBlockForRoundKey = errors.New("no proposed block")

// ErrBlockAlreadyExistsForRoundKey signals that there already exists a block for the RoundKey.
var ErrBlockAlreadyExistsForRoundKey = errors.New("block already exists for round")

// ErrVoteAlreadyExistsForSigner signals that a specific vote already exists.
var ErrVoteAlreadyExistsForSigner = errors.New("vote already exists for signer")

// ErrNoVotesForCurrentRoundKey signals that there are no votes for the current round.
var ErrNoVotesForCurrentRoundKey = errors.New("no votes for current round")

// ErrNilBlock signals a nil block.
var ErrNilBlock = errors.New("nil block")

// ErrNilVote signals a nil block.
var ErrNilVote = errors.New("nil vote")

// ErrInvalidValidator signals an invalid validator.
var ErrInvalidValidator = errors.New("invalid validator")

// ErrNilProposedBlockMessage signals a nil proposed block message
var ErrNilProposedBlockMessage = errors.New("nil proposed block message")

// ErrSignerIsNotValidator signals that the signer is not a validator
var ErrSignerIsNotValidator = errors.New("signer is not validator")

// ErrValidatorNotPartOfConsensusGroup signals that the validator is not part of the consensus group
var ErrValidatorNotPartOfConsensusGroup = errors.New("validator not part of consensus group")
