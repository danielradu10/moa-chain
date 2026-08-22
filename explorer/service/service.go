package service

import (
	"moa-chain/explorer"
	"moa-chain/explorer/resolvers"
)

// ExplorerService answers explorer queries. It owns no HTTP knowledge;
// controllers call its methods and write the results.
type ExplorerService struct {
	node          explorer.NodeFacade
	blockResolver resolvers.BlockResolver
	roundResolver resolvers.RoundResolver
}

// NewExplorerService creates a service wired to the given node.
func NewExplorerService(node explorer.NodeFacade) *ExplorerService {
	return &ExplorerService{
		node:          node,
		blockResolver: resolvers.NewBlockResolver(node),
		roundResolver: resolvers.NewRoundResolver(node),
	}
}

// GetHealth returns a snapshot of the node's current chain and round state.
func (s *ExplorerService) GetHealth() explorer.HealthResponse {
	round, _ := s.node.CurrentRound()
	miniRound, _ := s.node.CurrentMiniRound()
	epoch, _ := s.node.CurrentEpoch()

	return explorer.HealthResponse{
		Status:           "ok",
		ChainLength:      s.node.ChainLength(),
		CurrentRound:     round,
		CurrentMiniRound: miniRound,
		CurrentEpoch:     epoch,
	}
}

// GetBlock returns the block with the given hex-encoded header hash.
func (s *ExplorerService) GetBlock(hash string) (explorer.BlockResponse, bool) {
	return s.blockResolver.ResolveHash(hash)
}

// GetRound returns the state of the given round number.
func (s *ExplorerService) GetRound(round uint64) (explorer.RoundResponse, bool) {
	return s.roundResolver.Resolve(round)
}
