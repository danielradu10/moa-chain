package miniround1

import (
	"moa-chain/data"
)

// MiniRoundOneHandler defines what a MiniRound One Handler should do
type MiniRoundOneHandler interface {
	HandleProposingBlock(roundKey data.RoundKey) error
	HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error
	HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error
	HandleAggregatedVotes(roundKey data.RoundKey, votes *data.AggregatedVotes) error
}
