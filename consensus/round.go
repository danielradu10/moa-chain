package consensus

import (
	"log/slog"

	"moa-chain/consensus/miniround1"
	"moa-chain/data"
	"moa-chain/logging"
)

type roundHandler struct {
	selfID string

	currentStep         data.Step
	currentRoundKey     data.RoundKey
	miniRoundOneHandler miniround1.MiniRoundOneHandler
	logger              *slog.Logger
}

type RoundHandlerArgs struct {
	SelfID              string
	CurrentStep         data.Step
	CurrentRoundKey     data.RoundKey
	MiniRoundOneHandler miniround1.MiniRoundOneHandler
	Logger              *slog.Logger
}

// NewRoundHandler creates a new round handler
func NewRoundHandler(args RoundHandlerArgs) *roundHandler {
	return &roundHandler{
		selfID:              args.SelfID,
		currentStep:         args.CurrentStep,
		currentRoundKey:     args.CurrentRoundKey,
		miniRoundOneHandler: args.MiniRoundOneHandler,
		logger:              logging.FromOptional(args.Logger),
	}
}

func (rh *roundHandler) StartRound(roundKey data.RoundKey) error {
	rh.logger.Info("consensus.StartRound started", "roundKey", roundKey, "currentStep", rh.currentStep)
	rh.currentRoundKey = roundKey

	leaderID, err := rh.miniRoundOneHandler.HandleConsensusSelection(roundKey)
	if err != nil {
		rh.logger.Error("consensus.StartRound consensus selection failed", "roundKey", roundKey, "error", err)
		return err
	}
	rh.logger.Info("consensus.StartRound consensus selection completed", "roundKey", roundKey, "leaderID", leaderID)

	if leaderID == rh.selfID {
		rh.logger.Info("consensus.StartRound local node is leader; proposing block", "roundKey", roundKey)
		err = rh.miniRoundOneHandler.HandleProposingBlock(roundKey)
		if err != nil {
			rh.currentStep = data.StepFailed
			rh.logger.Error("consensus.StartRound block proposal failed", "roundKey", roundKey, "error", err)
			return err
		}

		rh.currentStep = data.StepCollectVotes
		rh.logger.Info("consensus.StartRound step changed", "roundKey", roundKey, "step", rh.currentStep)

		// return rh.timer.Start(roundKey, StepCollectVotes)
		return nil
	}

	rh.currentStep = data.StepAwaitProposal
	rh.logger.Info("consensus.StartRound step changed", "roundKey", roundKey, "step", rh.currentStep)
	// return rh.timer.Start(roundKey, StepAwaitProposal)

	return nil
}

func (rh *roundHandler) HandleMessage(message data.ConsensusMessage) error {
	rh.logger.Debug("consensus.HandleMessage handling consensus message", "messageType", message.ConsensusMessageType, "currentStep", rh.currentStep)
	switch message.ConsensusMessageType {
	case data.ProposedBlockConsensusMessage:
		return rh.handleProposedBlock(message)

	case data.BlockVoteConsensusMessage:
		return rh.handleBlockVote(message)

	case data.AggregatedVotesConsensusMessage:
		return rh.handleAggregatedVotes(message)

	default:
		return ErrUnknownConsensusMessage
	}
}

func (rh *roundHandler) handleProposedBlock(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepAwaitProposal {
		rh.logger.Error("unexpected proposed block message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	proposedBlock := message.ProposedBlockMessage
	if proposedBlock == nil {
		rh.logger.Error("proposed block message is nil")
		return ErrNilProposedBlockMessage
	}

	roundKey := data.RoundKey{
		Epoch:     proposedBlock.Epoch,
		Round:     proposedBlock.Round,
		MiniRound: proposedBlock.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("proposed block for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling proposed block", "roundKey", roundKey, "leaderID", proposedBlock.SenderID)
	err := rh.miniRoundOneHandler.HandleProposedBlock(roundKey, proposedBlock)
	if err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("proposed block handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.currentStep = data.StepAwaitAggregatedVotes
	rh.logger.Info("round step changed", "roundKey", roundKey, "step", rh.currentStep)
	// return rh.timer.Start(roundKey, StepAwaitAggregatedVotes)

	return nil
}

func (rh *roundHandler) handleBlockVote(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepCollectVotes {
		rh.logger.Error("unexpected block vote message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	vote := message.BlockVote
	if vote == nil {
		rh.logger.Error("block vote is nil")
		return ErrNilVote
	}

	roundKey := data.RoundKey{
		Epoch:     vote.Epoch,
		Round:     vote.Round,
		MiniRound: vote.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("block vote for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling block vote", "roundKey", roundKey, "signerID", vote.SignerID)
	return rh.miniRoundOneHandler.HandleBlockVote(roundKey, vote)
}

func (rh *roundHandler) handleAggregatedVotes(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepAwaitAggregatedVotes {
		rh.logger.Error("unexpected aggregated votes message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	votes := message.AggregatedVotes
	if votes == nil {
		rh.logger.Error("aggregated votes are nil")
		return ErrNilAggregatedVotes
	}

	roundKey := data.RoundKey{
		Epoch:     votes.Epoch,
		Round:     votes.Round,
		MiniRound: votes.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("aggregated votes for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling aggregated votes", "roundKey", roundKey, "senderID", votes.SenderID, "numSigners", len(votes.Signers))
	err := rh.miniRoundOneHandler.HandleAggregatedVotes(roundKey, votes)
	if err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("aggregated votes handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.currentStep = data.StepFinished
	rh.logger.Info("round finished", "roundKey", roundKey)
	//return rh.timer.Stop()
	return nil
}

func (rh *roundHandler) OnTimeout(roundKey data.RoundKey, step data.Step) error {
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("stale timeout for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey, "timeoutStep", step)
		return ErrStaleTimeout
	}

	if step != rh.currentStep {
		rh.logger.Error("stale timeout for different step", "roundKey", roundKey, "currentStep", rh.currentStep, "timeoutStep", step)
		return ErrStaleTimeout
	}

	rh.logger.Error("round timeout", "roundKey", roundKey, "step", step)
	switch step {
	case data.StepAwaitProposal:
		rh.currentStep = data.StepFailed
		return ErrProposalTimeout

	case data.StepCollectVotes:
		rh.currentStep = data.StepFailed
		return ErrNotEnoughVotes

	case data.StepAwaitAggregatedVotes:
		rh.currentStep = data.StepFailed
		return ErrAggregatedVotesTimeout

	default:
		return nil
	}
}
