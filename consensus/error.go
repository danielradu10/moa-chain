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

var ErrStaleTimeout = errors.New("stale timeout")

var ErrProposalTimeout = errors.New("proposal timeout")

var ErrNotEnoughVotes = errors.New("not enough votes")

var ErrAggregatedVotesTimeout = errors.New("aggregated votes timeout")
