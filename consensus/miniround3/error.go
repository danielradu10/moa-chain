package miniround3

import "errors"

var ErrNilProposedSynthesis = errors.New("miniround3: nil proposed synthesis message")
var ErrSynthesisProposalRoundKeyMismatch = errors.New("miniround3: synthesis proposal round key mismatch")
var ErrMessageNotFromLeader = errors.New("miniround3: message not from expected leader")
var ErrSynthesisBlockHashMismatch = errors.New("miniround3: synthesis block hash mismatch")
var ErrSynthesisTxCoverageMismatch = errors.New("miniround3: synthesis proposal does not cover expected eligible transactions")

var ErrNilSynthesisVote = errors.New("miniround3: nil synthesis vote")
var ErrOnlyLeaderCanCollectVotes = errors.New("miniround3: only the leader can collect synthesis votes")
var ErrSynthesisVoterNotInConsensusGroup = errors.New("miniround3: synthesis voter is not in the consensus group")
var ErrLeaderMustNotVote = errors.New("miniround3: the leader is excluded from casting a synthesis vote")
var ErrSynthesisVoteHashMismatch = errors.New("miniround3: synthesis vote hash mismatch")

var ErrNilAggregatedSynthesisVotes = errors.New("miniround3: nil aggregated synthesis votes")
var ErrAggregatedSynthesisVotesRoundKeyMismatch = errors.New("miniround3: aggregated synthesis votes round key mismatch")
var ErrParallelSliceLengthMismatch = errors.New("miniround3: aggregated synthesis votes parallel slice length mismatch")
var ErrNotEnoughSynthesisVotes = errors.New("miniround3: not enough synthesis votes for quorum")
