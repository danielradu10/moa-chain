package service

import (
	"encoding/hex"

	"moa-chain/data"
	"moa-chain/explorer"
	"moa-chain/explorer/resolvers"
)

// ExplorerService answers explorer queries. It owns no HTTP knowledge;
// controllers call its methods and write the results.
type ExplorerService struct {
	node          explorer.NodeFacade
	blockResolver resolvers.BlockResolver
	roundResolver resolvers.RoundResolver
	txResolver    resolvers.TxResolver
}

// NewExplorerService creates a service wired to the given node.
func NewExplorerService(node explorer.NodeFacade) *ExplorerService {
	return &ExplorerService{
		node:          node,
		blockResolver: resolvers.NewBlockResolver(node),
		roundResolver: resolvers.NewRoundResolver(node),
		txResolver:    resolvers.NewTxResolver(node),
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

// GetTransaction returns the full lifecycle state of a transaction by hex-encoded hash.
func (s *ExplorerService) GetTransaction(hash string) (explorer.TransactionResponse, bool) {
	return s.txResolver.Resolve(hash)
}

// GetTransactions returns all known transactions: finalized from the chain, then in-flight from the mempool.
func (s *ExplorerService) GetTransactions() []explorer.TransactionResponse {
	return s.txResolver.ResolveAll()
}

// GetLiveRound returns the current consensus step of the local node.
// Returns false if the round loop has not yet started.
func (s *ExplorerService) GetLiveRound() (explorer.LiveRoundResponse, bool) {
	key, step, ok := s.node.GetLiveRoundState()
	if !ok {
		return explorer.LiveRoundResponse{}, false
	}

	return explorer.LiveRoundResponse{
		Epoch:     key.Epoch,
		Round:     key.Round,
		MiniRound: key.MiniRound,
		Step:      stepName(step),
	}, true
}

// StreamLiveRound returns a channel of StepEvents for SSE streaming and an
// unsubscribe function. Returns false when the round hub is not wired.
func (s *ExplorerService) StreamLiveRound() (<-chan explorer.StepEvent, func(), bool) {
	return s.node.SubscribeLiveRound()
}

// StreamTxEvents returns a channel of TxEvents for SSE streaming on the given
// hex-encoded transaction hash and an unsubscribe function.
// Returns false when the tx hub is not wired or the hash is invalid.
func (s *ExplorerService) StreamTxEvents(hexHash string) (<-chan explorer.TxEvent, func(), bool) {
	hashBytes, err := hex.DecodeString(hexHash)
	if err != nil {
		return nil, nil, false
	}
	return s.node.SubscribeTxEvents(hashBytes)
}

// MakeLiveRoundResponse converts a StepEvent to an HTTP-ready LiveRoundResponse.
func (s *ExplorerService) MakeLiveRoundResponse(e explorer.StepEvent) explorer.LiveRoundResponse {
	return explorer.LiveRoundResponse{
		Epoch:     e.RoundKey.Epoch,
		Round:     e.RoundKey.Round,
		MiniRound: e.RoundKey.MiniRound,
		Step:      stepName(e.Step),
	}
}

func stepName(s data.Step) string {
	switch s {
	case data.StepIdle:
		return "IDLE"
	case data.StepSelectConsensusGroup:
		return "SELECT_CONSENSUS_GROUP"
	case data.StepAwaitProposal:
		return "AWAIT_PROPOSAL"
	case data.StepCollectVotes:
		return "COLLECT_VOTES"
	case data.StepAwaitAggregatedVotes:
		return "AWAIT_AGGREGATED_VOTES"
	case data.StepCollectExecutionResults:
		return "COLLECT_EXECUTION_RESULTS"
	case data.StepAwaitAnswerEvidence:
		return "AWAIT_ANSWER_EVIDENCE"
	case data.StepJudgeAnswers:
		return "JUDGE_ANSWERS"
	case data.StepCollectClassificationVotes:
		return "COLLECT_CLASSIFICATION_VOTES"
	case data.StepAwaitClassificationCertificate:
		return "AWAIT_CLASSIFICATION_CERTIFICATE"
	case data.StepSynthesizeAnswers:
		return "SYNTHESIZE_ANSWERS"
	case data.StepCollectSynthesisVotes:
		return "COLLECT_SYNTHESIS_VOTES"
	case data.StepAwaitProposedSynthesis:
		return "AWAIT_PROPOSED_SYNTHESIS"
	case data.StepAwaitAggregatedSynthesisVotes:
		return "AWAIT_AGGREGATED_SYNTHESIS_VOTES"
	case data.StepFinished:
		return "FINISHED"
	case data.StepFailed:
		return "FAILED"
	default:
		return "UNKNOWN"
	}
}
