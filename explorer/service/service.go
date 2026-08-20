package service

import "moa-chain/explorer"

// ExplorerService contains the query logic for the explorer API.
// No HTTP knowledge lives here — controllers call service methods and write the result.
type ExplorerService struct {
	node NodeFacade
}

func NewExplorerService(node NodeFacade) *ExplorerService {
	return &ExplorerService{node: node}
}

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
