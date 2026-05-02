package state

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
