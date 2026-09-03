package miniround3

import "moa-chain/data"

// MiniRoundThreeHandler defines what a handler for the third mini-round should do.
type MiniRoundThreeHandler interface {
	HandleConsensusSelection(key data.RoundKey) (leaderID string, err error)
	HandleSynthesis(key data.RoundKey) error
	HandleProposedSynthesis(key data.RoundKey, msg *data.ProposedSynthesisMessage) error
	HandleSynthesisVote(key data.RoundKey, vote *data.SynthesisVote) error
	HandleSynthesisApprovalGracePeriodElapsed(key data.RoundKey) error
	HandleAggregatedSynthesisVotes(key data.RoundKey, msg *data.AggregatedSynthesisVotes) error
}
