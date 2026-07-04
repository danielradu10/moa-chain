package consensus

import (
	"errors"
	"log/slog"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/consensus/miniround1"
	"moa-chain/consensus/miniround2"
	"moa-chain/data"
	"moa-chain/logging"
)

type roundHandler struct {
	selfID string

	currentStep         data.Step
	currentRoundKey     data.RoundKey
	miniRoundOneHandler miniround1.MiniRoundOneHandler
	miniRoundTwoHandler miniround2.MiniRoundTwoHandler
	blockFinalizer      blockFinalizer.BlockFinalizer
	logger              *slog.Logger
}

type RoundHandlerArgs struct {
	SelfID              string
	CurrentStep         data.Step
	CurrentRoundKey     data.RoundKey
	MiniRoundOneHandler miniround1.MiniRoundOneHandler
	MiniRoundTwoHandler miniround2.MiniRoundTwoHandler
	BlockFinalizer      blockFinalizer.BlockFinalizer
	Logger              *slog.Logger
}

// NewRoundHandler creates a new round handler
func NewRoundHandler(args RoundHandlerArgs) *roundHandler {
	return &roundHandler{
		selfID:              args.SelfID,
		currentStep:         args.CurrentStep,
		currentRoundKey:     args.CurrentRoundKey,
		miniRoundOneHandler: args.MiniRoundOneHandler,
		miniRoundTwoHandler: args.MiniRoundTwoHandler,
		blockFinalizer:      args.BlockFinalizer,
		logger:              logging.FromOptional(args.Logger),
	}
}

func (rh *roundHandler) StartRound(roundKey data.RoundKey) error {
	rh.logger.Info("consensus.StartRound started", "roundKey", roundKey, "currentStep", rh.currentStep)
	rh.currentRoundKey = roundKey

	switch roundKey.MiniRound {
	case uint64(data.MiniRoundTwo):
		return rh.startMiniRoundTwo(roundKey)

	default:
		return rh.startMiniRoundOne(roundKey)
	}
}

func (rh *roundHandler) startMiniRoundOne(roundKey data.RoundKey) error {
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

func (rh *roundHandler) startMiniRoundTwo(roundKey data.RoundKey) error {
	leaderID, err := rh.miniRoundTwoHandler.HandleConsensusSelection(roundKey)
	if err != nil {
		rh.logger.Error("consensus.StartRound mini-round two consensus selection failed", "roundKey", roundKey, "error", err)
		return err
	}
	rh.logger.Info("consensus.StartRound mini-round two consensus selection completed", "roundKey", roundKey, "leaderID", leaderID)

	err = rh.miniRoundTwoHandler.HandleBlockExecution(roundKey)
	if err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("consensus.StartRound mini-round two block execution failed", "roundKey", roundKey, "error", err)
		return err
	}

	if leaderID == rh.selfID {
		rh.currentStep = data.StepCollectExecutionResults
	} else {
		rh.currentStep = data.StepAwaitAggregatedExecutionResults
	}

	rh.logger.Info("consensus.StartRound mini-round two step changed", "roundKey", roundKey, "step", rh.currentStep)
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

	case data.ExecutedPromptsMessage:
		return rh.handleExecutedPrompts(message)

	case data.AggregatedExecutionResultsConsensusMessage:
		return rh.handleAggregatedExecutionResults(message)

	case data.AnswerClassificationVoteConsensusMessage:
		return rh.handleAnswerClassificationVote(message)

	case data.AnswerClassificationCertificateConsensusMessage:
		return rh.handleAnswerClassificationCertificate(message)

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

	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring block vote for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}

	if rh.currentStep != data.StepCollectVotes {
		rh.logger.Error("unexpected block vote message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	if roundKey != rh.currentRoundKey {
		if rh.isFinalizedPreviousMiniRound(roundKey) {
			rh.logger.Info("ignoring stale block vote for finalized previous mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
			return nil
		}

		rh.logger.Error("block vote for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling block vote", "roundKey", roundKey, "signerID", vote.SignerID)
	err := rh.miniRoundOneHandler.HandleBlockVote(roundKey, vote)
	if err != nil {
		return err
	}

	return rh.startMiniRoundTwoIfMiniRoundOneWasFinalized(roundKey)
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
		if rh.isFinalizedPreviousMiniRound(roundKey) {
			rh.logger.Info("ignoring stale aggregated votes for finalized previous mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
			return nil
		}

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

	return rh.startNextMiniRound(roundKey)
}

func (rh *roundHandler) startMiniRoundTwoIfMiniRoundOneWasFinalized(roundKey data.RoundKey) error {
	_, err := rh.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
	if err != nil {
		if errors.Is(err, blockFinalizer.ErrFinalizedBlockNotFound) {
			return nil
		}

		rh.currentStep = data.StepFailed
		rh.logger.Error("failed to check mini-round one finalization", "roundKey", roundKey, "error", err)
		return err
	}

	return rh.startNextMiniRound(roundKey)
}

func (rh *roundHandler) startNextMiniRound(roundKey data.RoundKey) error {
	nextRoundKey := data.RoundKey{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound + 1,
	}

	if nextRoundKey.MiniRound == uint64(data.MiniRoundTwo) {
		finalizedBlock, err := rh.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		if err != nil {
			rh.currentStep = data.StepFailed
			rh.logger.Error("failed to get finalized mini-round one block before starting mini-round two", "roundKey", roundKey, "error", err)
			return err
		}

		if len(finalizedBlock.SubdomainsFrequencies) == 0 {
			rh.currentStep = data.StepFinished
			rh.logger.Info("mini-round one finalized without subdomain frequencies; skipping mini-round two", "roundKey", roundKey)
			return nil
		}
	}

	rh.logger.Info("starting next mini-round", "currentRoundKey", roundKey, "nextRoundKey", nextRoundKey)
	return rh.StartRound(nextRoundKey)
}

func (rh *roundHandler) isFinalizedPreviousMiniRound(roundKey data.RoundKey) bool {
	if roundKey.Epoch != rh.currentRoundKey.Epoch ||
		roundKey.Round != rh.currentRoundKey.Round ||
		roundKey.MiniRound+1 != rh.currentRoundKey.MiniRound {
		return false
	}

	_, err := rh.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
	return err == nil
}

func (rh *roundHandler) shouldIgnoreFinalizedMiniRoundMessage(roundKey data.RoundKey) bool {
	if roundKey == rh.currentRoundKey {
		return rh.currentStep == data.StepFinished && rh.isFinalizedRoundKey(roundKey)
	}

	return rh.isFinalizedPreviousMiniRound(roundKey)
}

func (rh *roundHandler) isFinalizedRoundKey(roundKey data.RoundKey) bool {
	switch roundKey.MiniRound {
	case uint64(data.MiniRoundOne):
		_, err := rh.blockFinalizer.GetFinalizedBlockInMROne(roundKey)
		return err == nil

	case uint64(data.MiniRoundTwo):
		_, err := rh.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey)
		return err == nil

	default:
		return false
	}
}

func (rh *roundHandler) handleExecutedPrompts(message data.ConsensusMessage) error {
	executedPrompts := message.ExecutedPrompts
	if executedPrompts == nil {
		rh.logger.Error("executed prompts message is nil")
		return ErrNilExecutedPrompts
	}

	roundKey := data.RoundKey{
		Epoch:     executedPrompts.Epoch,
		Round:     executedPrompts.Round,
		MiniRound: executedPrompts.MiniRound,
	}

	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring executed prompts for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}

	if rh.currentStep != data.StepCollectExecutionResults {
		rh.logger.Error("unexpected executed prompts message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("executed prompts for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling executed prompts", "roundKey", roundKey, "senderID", executedPrompts.SenderID)
	err := rh.miniRoundTwoHandler.HandleExecutedPromptsMessage(roundKey, executedPrompts)
	if err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("executed prompts handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	_, err = rh.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey)
	if err == nil {
		rh.currentStep = data.StepFinished
		rh.logger.Info("mini-round two leader finished", "roundKey", roundKey)
		return nil
	}
	if !errors.Is(err, blockFinalizer.ErrFinalizedBlockNotFound) {
		rh.currentStep = data.StepFailed
		rh.logger.Error("failed to check mini-round two finalization", "roundKey", roundKey, "error", err)
		return err
	}

	return nil
}

func (rh *roundHandler) handleAggregatedExecutionResults(message data.ConsensusMessage) error {
	aggregatedExecutionResults := message.AggregatedExecutionResults
	if aggregatedExecutionResults == nil {
		rh.logger.Error("aggregated execution results are nil")
		return ErrNilAggregatedExecutionResults
	}

	roundKey := data.RoundKey{
		Epoch:     aggregatedExecutionResults.Epoch,
		Round:     aggregatedExecutionResults.Round,
		MiniRound: aggregatedExecutionResults.MiniRound,
	}

	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring aggregated execution results for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}

	if rh.currentStep == data.StepAwaitAnswerEvidence {
		return rh.handleAnswerEvidenceForClassification(roundKey, aggregatedExecutionResults)
	}

	if rh.currentStep != data.StepAwaitAggregatedExecutionResults {
		rh.logger.Error("unexpected aggregated execution results message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("aggregated execution results for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling aggregated execution results", "roundKey", roundKey, "senderID", aggregatedExecutionResults.SenderID, "numSigners", len(aggregatedExecutionResults.Signers))
	err := rh.miniRoundTwoHandler.HandleAggregatedExecutionResults(roundKey, aggregatedExecutionResults)
	if err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("aggregated execution results handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.currentStep = data.StepFinished
	rh.logger.Info("mini-round two finished", "roundKey", roundKey)
	return nil
}

// handleAnswerEvidenceForClassification activates judging only for the new,
// explicitly selected answer-evidence step; the legacy finalization path remains separate.
func (rh *roundHandler) handleAnswerEvidenceForClassification(
	roundKey data.RoundKey,
	evidence *data.AggregatedExecutionResultsMessage,
) error {
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("answer evidence for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.currentStep = data.StepJudgeAnswers
	rh.logger.Info("judging verified answer evidence", "roundKey", roundKey, "senderID", evidence.SenderID)
	if err := rh.miniRoundTwoHandler.HandleAnswerEvidenceForClassification(roundKey, evidence); err != nil {
		rh.currentStep = data.StepFailed
		rh.logger.Error("answer evidence judging failed", "roundKey", roundKey, "error", err)
		return err
	}

	if evidence.SenderID == rh.selfID {
		rh.currentStep = data.StepCollectClassificationVotes
	} else {
		rh.currentStep = data.StepAwaitClassificationCertificate
	}

	rh.logger.Info("answer classification vote produced", "roundKey", roundKey, "step", rh.currentStep)
	return nil
}

func (rh *roundHandler) handleAnswerClassificationVote(message data.ConsensusMessage) error {
	vote := message.AnswerClassificationVote
	if vote == nil {
		rh.logger.Error("answer classification vote is nil")
		return ErrNilAnswerClassificationVote
	}

	roundKey := data.RoundKey{Epoch: vote.Epoch, Round: vote.Round, MiniRound: vote.MiniRound}
	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring classification vote for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}
	if rh.currentStep != data.StepCollectClassificationVotes {
		rh.logger.Error("unexpected classification vote for step", "currentStep", rh.currentStep, "judgeID", vote.JudgeID)
		return ErrUnexpectedMessageForStep
	}
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("classification vote for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey, "judgeID", vote.JudgeID)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info(
		"handling answer classification vote",
		"roundKey", roundKey,
		"judgeID", vote.JudgeID,
		"answerEvidenceHash", vote.AnswerEvidenceHash,
		"promptVersion", vote.PromptVersion,
	)
	return rh.miniRoundTwoHandler.HandleAnswerClassificationVote(roundKey, vote)
}

func (rh *roundHandler) handleAnswerClassificationCertificate(message data.ConsensusMessage) error {
	certificate := message.AnswerClassificationCertificate
	if certificate == nil {
		rh.logger.Error("answer classification certificate is nil")
		return ErrNilAnswerClassificationCertificate
	}

	roundKey := data.RoundKey{
		Epoch: certificate.Epoch, Round: certificate.Round, MiniRound: certificate.MiniRound,
	}
	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring classification certificate for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}
	if rh.currentStep != data.StepAwaitClassificationCertificate {
		rh.logger.Error("unexpected classification certificate for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("classification certificate for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info(
		"handling answer classification certificate",
		"roundKey", roundKey,
		"senderID", certificate.SenderID,
		"answerEvidenceHash", certificate.AnswerEvidenceHash,
		"promptVersion", certificate.PromptVersion,
	)
	return rh.miniRoundTwoHandler.HandleAnswerClassificationCertificate(roundKey, certificate)
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

	case data.StepCollectExecutionResults:
		rh.currentStep = data.StepFailed
		return ErrNotEnoughExecutionResults

	case data.StepAwaitAggregatedExecutionResults:
		rh.currentStep = data.StepFailed
		return ErrAggregatedExecutionResultsTimeout

	case data.StepAwaitAnswerEvidence:
		rh.currentStep = data.StepFailed
		return ErrAnswerEvidenceTimeout

	case data.StepJudgeAnswers:
		rh.currentStep = data.StepFailed
		return ErrAnswerJudgingTimeout

	case data.StepCollectClassificationVotes:
		rh.currentStep = data.StepFailed
		return ErrNotEnoughAnswerClassificationVotes

	case data.StepAwaitClassificationCertificate:
		rh.currentStep = data.StepFailed
		return ErrAnswerClassificationCertificateTimeout

	default:
		return nil
	}
}
