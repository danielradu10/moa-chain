package resolvers

import (
	"encoding/hex"

	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/tracker"
)

// RoundResolver looks up round state from the chain or the round tracker.
type RoundResolver interface {
	// Resolve returns the state of a round by number. It checks finalized chain
	// blocks first, then falls back to the RoundTracker for in-progress rounds.
	Resolve(round uint64) (explorer.RoundResponse, bool)
	// ResolveAll returns a summary of all known rounds, newest first.
	ResolveAll() []explorer.RoundSummary
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

func (r *roundResolver) ResolveAll() []explorer.RoundSummary {
	seen := make(map[uint64]struct{})
	var result []explorer.RoundSummary

	all := r.node.AllBlocks()
	for i := len(all) - 1; i >= 0; i-- {
		block := all[i]
		round := block.Header.Round
		if _, ok := seen[round]; ok {
			continue
		}
		seen[round] = struct{}{}
		result = append(result, explorer.RoundSummary{
			Round:     round,
			Epoch:     block.Header.Epoch,
			Status:    "FINALIZED",
			TxCount:   len(block.Body.Transactions),
			BlockHash: hex.EncodeToString(block.Header.HeaderHash),
		})
	}

	epoch, _ := r.node.CurrentEpoch()
	currentRound, _ := r.node.CurrentRound()
	if _, ok := seen[currentRound]; !ok {
		if entry, ok := r.node.GetRound(epoch, currentRound); ok {
			result = append([]explorer.RoundSummary{{
				Round:  entry.Round,
				Epoch:  entry.Epoch,
				Status: string(entry.Status),
				TxCount: func() int {
					if entry.MR1Block != nil {
						return len(entry.MR1Block.Body.Transactions)
					}
					return 0
				}(),
			}}, result...)
		}
	}

	return result
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
