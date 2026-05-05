package consensus

import (
	"moa-chain/consensus/miniround1"
	"moa-chain/data"
)

type roundHandler struct {
	selfID string

	currentStep         data.Step
	currentRoundKey     data.RoundKey
	miniRoundOneHandler miniround1.MiniRoundOneHandler
}

func (rh *roundHandler) StartRound(roundKey data.RoundKey) error {
	rh.currentRoundKey = roundKey

	leaderID, err := rh.miniRoundOneHandler.HandleConsensusSelection(roundKey)
	if err != nil {
		return err
	}

	if leaderID == rh.selfID {
		err = rh.miniRoundOneHandler.HandleProposingBlock(roundKey)
		if err != nil {
			rh.currentStep = data.StepFailed
			return err
		}

		rh.currentStep = data.StepCollectVotes

		// return rh.timer.Start(roundKey, StepCollectVotes)
		return nil
	}

	rh.currentStep = data.StepAwaitProposal
	// return rh.timer.Start(roundKey, StepAwaitProposal)

	return nil
}

func (rh *roundHandler) HandleMessage(message data.ConsensusMessage) error {
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
		return ErrUnexpectedMessageForStep
	}

	proposedBlock := message.ProposedBlockMessage
	if proposedBlock == nil {
		return ErrNilProposedBlockMessage
	}

	roundKey := data.RoundKey{
		Epoch:     proposedBlock.Epoch,
		Round:     proposedBlock.Round,
		MiniRound: proposedBlock.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		return ErrMessageForDifferentRound
	}

	err := rh.miniRoundOneHandler.HandleProposedBlock(roundKey, proposedBlock)
	if err != nil {
		rh.currentStep = data.StepFailed
		return err
	}

	rh.currentStep = data.StepAwaitAggregatedVotes
	// return rh.timer.Start(roundKey, StepAwaitAggregatedVotes)

	return nil
}

func (rh *roundHandler) handleBlockVote(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepCollectVotes {
		return ErrUnexpectedMessageForStep
	}

	vote := message.BlockVote
	if vote == nil {
		return ErrNilVote
	}

	roundKey := data.RoundKey{
		Epoch:     vote.Epoch,
		Round:     vote.Round,
		MiniRound: vote.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		return ErrMessageForDifferentRound
	}

	return rh.miniRoundOneHandler.HandleBlockVote(roundKey, vote)
}

func (rh *roundHandler) handleAggregatedVotes(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepAwaitAggregatedVotes {
		return ErrUnexpectedMessageForStep
	}

	votes := message.AggregatedVotes
	if votes == nil {
		return ErrNilAggregatedVotes
	}

	roundKey := data.RoundKey{
		Epoch:     votes.Epoch,
		Round:     votes.Round,
		MiniRound: votes.MiniRound,
	}

	if roundKey != rh.currentRoundKey {
		return ErrMessageForDifferentRound
	}

	err := rh.miniRoundOneHandler.HandleAggregatedVotes(roundKey, votes)
	if err != nil {
		rh.currentStep = data.StepFailed
		return err
	}

	rh.currentStep = data.StepFinished
	//return rh.timer.Stop()
	return nil
}

func (rh *roundHandler) OnTimeout(roundKey data.RoundKey, step data.Step) error {
	if roundKey != rh.currentRoundKey {
		return ErrStaleTimeout
	}

	if step != rh.currentStep {
		return ErrStaleTimeout
	}

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
