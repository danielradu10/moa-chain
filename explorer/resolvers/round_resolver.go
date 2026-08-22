package resolvers

import (
	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/tracker"
)

// RoundResolver looks up round state from the chain or the round tracker.
type RoundResolver interface {
	// Resolve returns the state of a round by number. It checks finalized chain
	// blocks first, then falls back to the RoundTracker for in-progress rounds.
	Resolve(round uint64) (explorer.RoundResponse, bool)
}

type roundResolver struct {
	node   explorer.NodeFacade
	blocks *blockResolver
}

// NewRoundResolver returns a RoundResolver backed by the given node.
func NewRoundResolver(node explorer.NodeFacade) RoundResolver {
	return &roundResolver{
		node:   node,
		blocks: &blockResolver{node: node},
	}
}

func (r *roundResolver) Resolve(round uint64) (explorer.RoundResponse, bool) {
	if chainBlock, ok := r.blocks.byRound(round); ok && chainBlock != nil {
		return r.fromChainBlock(chainBlock), true
	}

	epoch, _ := r.node.CurrentEpoch()
	entry, ok := r.node.GetRound(epoch, round)
	if !ok {
		return explorer.RoundResponse{}, false
	}

	return r.fromTrackerEntry(entry), true
}

func (r *roundResolver) fromChainBlock(block *data.BlockOnChain) explorer.RoundResponse {
	resp := explorer.RoundResponse{
		Round:  block.Header.Round,
		Epoch:  block.Header.Epoch,
		Status: "FINALIZED",
		MR1:    toMR1Response(block),
	}

	if len(block.AnswerClassifications) > 0 {
		resp.MR2 = toMR2Response(block)
	}

	if len(block.FinalAnswers) > 0 {
		resp.MR3 = toMR3Response(block)
	}

	return resp
}

func (r *roundResolver) fromTrackerEntry(entry tracker.RoundEntry) explorer.RoundResponse {
	return explorer.RoundResponse{
		Round:  entry.Round,
		Epoch:  entry.Epoch,
		Status: string(entry.Status),
		MR1:    toMR1Response(entry.MR1Block),
		MR2:    toMR2Response(entry.MR2Block),
		MR3:    toMR3Response(entry.MR3Block),
	}
}
