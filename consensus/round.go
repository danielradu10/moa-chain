package consensus

import (
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/chain"
	"moa-chain/consensus/miniround1"
	"moa-chain/consensus/miniround2"
	"moa-chain/consensus/miniround3"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/txpipeline"
)

// roundStateSnapshot is the atomically stored live state of the round handler.
type roundStateSnapshot struct {
	roundKey data.RoundKey
	step     data.Step
}

type roundHandler struct {
	selfID string

	currentStep             data.Step
	currentRoundKey         data.RoundKey
	snapshot                atomic.Pointer[roundStateSnapshot]
	onStepChanged           func(data.RoundKey, data.Step)
	miniRoundOneHandler     miniround1.MiniRoundOneHandler
	miniRoundTwoHandler     miniround2.MiniRoundTwoHandler
	miniRoundThreeHandler   miniround3.MiniRoundThreeHandler
	blockFinalizer          blockFinalizer.BlockFinalizer
	chain                   chain.Chain
	mempool                 mempool.Mempool
	store                   txpipeline.PrecomputedStore
	roundState              state.RoundState
	blockchainState         state.BlockchainState
	stopAfterMiniRoundOne   bool
	stopAfterMiniRoundTwo   bool
	logger                  *slog.Logger

	// inbox is the write end of this node's event queue, used to self-inject
	// TimeoutEvents from background timer goroutines.
	inbox chan<- data.RoundEvent
	// voteCollectionDeadline is how long the MR1 leader waits for all G consensus-group
	// votes before falling back to aggregating with whatever Q+ votes arrived.
	// Zero disables the fallback timer (leader waits for G votes indefinitely).
	voteCollectionDeadline time.Duration
	// executionResultsCollectionDeadline is how long the MR2 leader waits for all G
	// execution results before falling back to aggregating with whatever Q+ arrived.
	// Zero disables the fallback timer (leader waits indefinitely).
	executionResultsCollectionDeadline time.Duration
	// miniRoundDuration is a fixed delay inserted before each mini-round transition
	// so that every mini-round takes at least this long regardless of whether
	// transactions are present. Zero means advance immediately (suitable for tests).
	miniRoundDuration time.Duration
}

type RoundHandlerArgs struct {
	SelfID                string
	CurrentStep           data.Step
	CurrentRoundKey       data.RoundKey
	MiniRoundOneHandler   miniround1.MiniRoundOneHandler
	MiniRoundTwoHandler   miniround2.MiniRoundTwoHandler
	MiniRoundThreeHandler miniround3.MiniRoundThreeHandler
	BlockFinalizer        blockFinalizer.BlockFinalizer
	Chain                 chain.Chain
	Mempool               mempool.Mempool
	Store                 txpipeline.PrecomputedStore
	RoundState            state.RoundState
	BlockchainState       state.BlockchainState
	// StopAfterMiniRoundOne prevents the handler from advancing to MR2 after MR1
	// finalization. Intended for MR1-only tests and benchmarks.
	StopAfterMiniRoundOne bool
	// StopAfterMiniRoundTwo prevents the handler from advancing to MR3 after MR2
	// finalization. Intended for MR2-only tests.
	StopAfterMiniRoundTwo bool
	Logger                *slog.Logger

	// Inbox is the node's event channel. Required when VoteCollectionDeadline > 0.
	Inbox chan data.RoundEvent
	// VoteCollectionDeadline is the fallback timeout for the MR1 leader's vote-collection
	// step. Zero means wait for all G votes with no timeout.
	VoteCollectionDeadline time.Duration
	// ExecutionResultsCollectionDeadline is the fallback timeout for the MR2 leader's
	// execution-results collection step. Zero means wait for all G results with no timeout.
	ExecutionResultsCollectionDeadline time.Duration
	// MiniRoundDuration is a fixed slot enforced between mini-round transitions so that
	// every mini-round takes at least this long regardless of whether transactions are
	// present. Zero means advance immediately (suitable for tests).
	MiniRoundDuration time.Duration
	// OnStepChanged is called on every step transition. Used by the explorer to push
	// live-round events to SSE subscribers. Optional; nil disables callbacks.
	OnStepChanged func(data.RoundKey, data.Step)
}

// NewRoundHandler creates a new round handler
func NewRoundHandler(args RoundHandlerArgs) *roundHandler {
	return &roundHandler{
		selfID:                             args.SelfID,
		currentStep:                        args.CurrentStep,
		currentRoundKey:                    args.CurrentRoundKey,
		miniRoundOneHandler:                args.MiniRoundOneHandler,
		miniRoundTwoHandler:                args.MiniRoundTwoHandler,
		miniRoundThreeHandler:              args.MiniRoundThreeHandler,
		blockFinalizer:                     args.BlockFinalizer,
		chain:                              args.Chain,
		mempool:                            args.Mempool,
		store:                              args.Store,
		roundState:                         args.RoundState,
		blockchainState:                    args.BlockchainState,
		stopAfterMiniRoundOne:              args.StopAfterMiniRoundOne,
		stopAfterMiniRoundTwo:              args.StopAfterMiniRoundTwo,
		logger:                             logging.FromOptional(args.Logger),
		inbox:                              args.Inbox,
		voteCollectionDeadline:             args.VoteCollectionDeadline,
		executionResultsCollectionDeadline: args.ExecutionResultsCollectionDeadline,
		miniRoundDuration:                  args.MiniRoundDuration,
		onStepChanged:                      args.OnStepChanged,
	}
}

func (rh *roundHandler) StartRound(roundKey data.RoundKey) error {
	rh.logger.Info("consensus.StartRound started", "roundKey", roundKey, "currentStep", rh.currentStep)
	rh.currentRoundKey = roundKey

	switch roundKey.MiniRound {
	case uint64(data.MiniRoundTwo):
		return rh.startMiniRoundTwo(roundKey)

	case uint64(data.MiniRoundThree):
		return rh.startMiniRoundThree(roundKey)

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
			rh.updateCurrentStep(data.StepFailed)
			rh.logger.Error("consensus.StartRound block proposal failed", "roundKey", roundKey, "error", err)
			return err
		}

		rh.updateCurrentStep(data.StepCollectVotes)
		rh.logger.Info("consensus.StartRound step changed", "roundKey", roundKey, "step", rh.currentStep)

		if rh.voteCollectionDeadline > 0 {
			go rh.scheduleVoteCollectionTimeout(roundKey)
		}
		return nil
	}

	rh.updateCurrentStep(data.StepAwaitProposal)
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
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("consensus.StartRound mini-round two block execution failed", "roundKey", roundKey, "error", err)
		return err
	}

	if leaderID == rh.selfID {
		rh.updateCurrentStep(data.StepCollectExecutionResults)
		if rh.executionResultsCollectionDeadline > 0 {
			go rh.scheduleExecutionResultsCollectionTimeout(roundKey)
		}
	} else {
		rh.updateCurrentStep(data.StepAwaitAnswerEvidence)
	}

	rh.logger.Info("consensus.StartRound mini-round two step changed", "roundKey", roundKey, "step", rh.currentStep)
	return nil
}

func (rh *roundHandler) startMiniRoundThree(roundKey data.RoundKey) error {
	leaderID, err := rh.miniRoundThreeHandler.HandleConsensusSelection(roundKey)
	if err != nil {
		rh.logger.Error("consensus.StartRound mini-round three consensus selection failed", "roundKey", roundKey, "error", err)
		return err
	}
	rh.logger.Info("consensus.StartRound mini-round three consensus selection completed", "roundKey", roundKey, "leaderID", leaderID)

	if leaderID == rh.selfID {
		if err = rh.miniRoundThreeHandler.HandleSynthesis(roundKey); err != nil {
			rh.updateCurrentStep(data.StepFailed)
			rh.logger.Error("consensus.StartRound mini-round three synthesis failed", "roundKey", roundKey, "error", err)
			return err
		}
		rh.updateCurrentStep(data.StepCollectSynthesisVotes)
	} else {
		rh.updateCurrentStep(data.StepAwaitProposedSynthesis)
	}

	rh.logger.Info("consensus.StartRound mini-round three step set", "roundKey", roundKey, "step", rh.currentStep)
	return nil
}

// onMiniRoundTwoFinalized advances to mini-round three when a handler is wired
// up and StopAfterMiniRoundTwo is false; otherwise marks the round finished.
func (rh *roundHandler) onMiniRoundTwoFinalized(roundKey data.RoundKey) error {
	if rh.miniRoundThreeHandler == nil || rh.stopAfterMiniRoundTwo {
		rh.updateCurrentStep(data.StepFinished)
		rh.logger.Info("mini-round two finalized; stopping before mini-round three", "roundKey", roundKey)
		return nil
	}
	return rh.startNextMiniRound(roundKey)
}

// scheduleVoteCollectionTimeout fires a TimeoutEvent for StepCollectVotes after
// voteCollectionDeadline elapses. If the leader already collected all G votes the
// event will be treated as stale and silently ignored by OnTimeout.
func (rh *roundHandler) scheduleVoteCollectionTimeout(roundKey data.RoundKey) {
	time.Sleep(rh.voteCollectionDeadline)
	if rh.inbox == nil {
		return
	}
	rh.inbox <- data.RoundEvent{
		Type: data.TimeoutEvent,
		Timeout: &data.RoundTimeout{
			RoundKey: roundKey,
			Step:     data.StepCollectVotes,
		},
	}
}

func (rh *roundHandler) scheduleExecutionResultsCollectionTimeout(roundKey data.RoundKey) {
	time.Sleep(rh.executionResultsCollectionDeadline)
	if rh.inbox == nil {
		return
	}
	rh.inbox <- data.RoundEvent{
		Type: data.TimeoutEvent,
		Timeout: &data.RoundTimeout{
			RoundKey: roundKey,
			Step:     data.StepCollectExecutionResults,
		},
	}
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

	case data.AnswerEvidenceConsensusMessage:
		return rh.handleAnswerEvidence(message)

	case data.AnswerClassificationVoteConsensusMessage:
		return rh.handleAnswerClassificationVote(message)

	case data.AnswerClassificationCertificateConsensusMessage:
		return rh.handleAnswerClassificationCertificate(message)

	case data.ProposedSynthesisConsensusMessage:
		return rh.handleProposedSynthesis(message)

	case data.SynthesisVoteConsensusMessage:
		return rh.handleSynthesisVote(message)

	case data.AggregatedSynthesisVotesConsensusMessage:
		return rh.handleAggregatedSynthesisVotes(message)

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
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("proposed block handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.updateCurrentStep(data.StepAwaitAggregatedVotes)
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
		rh.updateCurrentStep(data.StepFailed)
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

		rh.updateCurrentStep(data.StepFailed)
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
		if rh.stopAfterMiniRoundOne {
			rh.updateCurrentStep(data.StepFinished)
			rh.logger.Info("mini-round one finalized; stopping before mini-round two (StopAfterMiniRoundOne=true)", "roundKey", roundKey)
			return nil
		}
	}

	rh.logger.Info("starting next mini-round", "currentRoundKey", roundKey, "nextRoundKey", nextRoundKey)
	if rh.miniRoundDuration > 0 {
		time.Sleep(rh.miniRoundDuration)
	}
	return rh.StartRound(nextRoundKey)
}

func (rh *roundHandler) isFinalizedPreviousMiniRound(roundKey data.RoundKey) bool {
	if roundKey.Epoch != rh.currentRoundKey.Epoch ||
		roundKey.Round != rh.currentRoundKey.Round ||
		roundKey.MiniRound+1 != rh.currentRoundKey.MiniRound {
		return false
	}

	return rh.isFinalizedRoundKey(roundKey)
}

func (rh *roundHandler) shouldIgnoreFinalizedMiniRoundMessage(roundKey data.RoundKey) bool {
	if roundKey == rh.currentRoundKey {
		return rh.currentStep == data.StepFinished && rh.isFinalizedRoundKey(roundKey)
	}

	// Messages from a previous epoch or round are always stale; ignore them
	// rather than returning an error (the round was finalized and the round
	// state/block-finalizer entries were already pruned by finalizeRound).
	if roundKey.Epoch < rh.currentRoundKey.Epoch ||
		(roundKey.Epoch == rh.currentRoundKey.Epoch && roundKey.Round < rh.currentRoundKey.Round) {
		return true
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

	case uint64(data.MiniRoundThree):
		_, err := rh.blockFinalizer.GetFinalizedBlockInMRThree(roundKey)
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
	if rh.currentStep == data.StepCollectClassificationVotes && roundKey == rh.currentRoundKey {
		rh.logger.Info("ignoring late execution result after classification started", "roundKey", roundKey, "senderID", executedPrompts.SenderID)
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
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("executed prompts handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	_, err = rh.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey)
	if err == nil {
		rh.logger.Info("mini-round two leader finished", "roundKey", roundKey)
		return rh.onMiniRoundTwoFinalized(roundKey)
	}
	if !errors.Is(err, blockFinalizer.ErrFinalizedBlockNotFound) {
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("failed to check mini-round two finalization", "roundKey", roundKey, "error", err)
		return err
	}
	if rh.miniRoundTwoHandler.HasVerifiedAnswerEvidence(roundKey) {
		rh.updateCurrentStep(data.StepCollectClassificationVotes)
		rh.logger.Info("mini-round two leader collecting classification votes", "roundKey", roundKey)
	}

	return nil
}

func (rh *roundHandler) handleAnswerEvidence(message data.ConsensusMessage) error {
	answerEvidence := message.AnswerEvidence
	if answerEvidence == nil {
		rh.logger.Error("answer evidence is nil")
		return ErrNilAnswerEvidence
	}

	roundKey := data.RoundKey{
		Epoch:     answerEvidence.Epoch,
		Round:     answerEvidence.Round,
		MiniRound: answerEvidence.MiniRound,
	}

	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring answer evidence for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}

	if rh.currentStep != data.StepAwaitAnswerEvidence {
		rh.logger.Error("unexpected answer evidence message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("answer evidence for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.updateCurrentStep(data.StepJudgeAnswers)
	rh.logger.Info("judging verified answer evidence", "roundKey", roundKey, "senderID", answerEvidence.SenderID)
	if err := rh.miniRoundTwoHandler.HandleAnswerEvidence(roundKey, answerEvidence); err != nil {
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("answer evidence judging failed", "roundKey", roundKey, "error", err)
		return err
	}

	if answerEvidence.SenderID == rh.selfID {
		rh.updateCurrentStep(data.StepCollectClassificationVotes)
	} else {
		rh.updateCurrentStep(data.StepAwaitClassificationCertificate)
	}

	rh.logger.Info("answer classification vote produced", "roundKey", roundKey, "step", rh.currentStep)

	if rh.isFinalizedRoundKey(roundKey) {
		return rh.onMiniRoundTwoFinalized(roundKey)
	}
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
	if err := rh.miniRoundTwoHandler.HandleAnswerClassificationVote(roundKey, vote); err != nil {
		return err
	}
	if rh.isFinalizedRoundKey(roundKey) {
		return rh.onMiniRoundTwoFinalized(roundKey)
	}

	return nil
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
	if err := rh.miniRoundTwoHandler.HandleAnswerClassificationCertificate(roundKey, certificate); err != nil {
		rh.updateCurrentStep(data.StepFailed)
		return err
	}

	return rh.onMiniRoundTwoFinalized(roundKey)
}

func (rh *roundHandler) OnTimeout(roundKey data.RoundKey, step data.Step) error {
	if roundKey != rh.currentRoundKey {
		rh.logger.Info("stale timeout for different round (ignoring)", "currentRoundKey", rh.currentRoundKey, "timeoutRoundKey", roundKey, "timeoutStep", step)
		return nil
	}

	if step != rh.currentStep {
		rh.logger.Info("stale timeout for different step (ignoring)", "roundKey", roundKey, "currentStep", rh.currentStep, "timeoutStep", step)
		return nil
	}

	rh.logger.Info("round timeout", "roundKey", roundKey, "step", step)
	switch step {
	case data.StepAwaitProposal:
		rh.updateCurrentStep(data.StepFailed)
		return ErrProposalTimeout

	case data.StepCollectVotes:
		err := rh.miniRoundOneHandler.HandleVoteCollectionTimeout(roundKey)
		if err != nil {
			rh.updateCurrentStep(data.StepFailed)
			return err
		}
		return rh.startMiniRoundTwoIfMiniRoundOneWasFinalized(roundKey)

	case data.StepAwaitAggregatedVotes:
		rh.updateCurrentStep(data.StepFailed)
		return ErrAggregatedVotesTimeout

	case data.StepCollectExecutionResults:
		err := rh.miniRoundTwoHandler.HandleExecutedPromptsCollectionTimeout(roundKey)
		if err != nil {
			rh.updateCurrentStep(data.StepFailed)
			return err
		}
		_, err = rh.blockFinalizer.GetFinalizedBlockInMRTwo(roundKey)
		if err == nil {
			return rh.onMiniRoundTwoFinalized(roundKey)
		}
		if rh.miniRoundTwoHandler.HasVerifiedAnswerEvidence(roundKey) {
			rh.updateCurrentStep(data.StepCollectClassificationVotes)
		}
		return nil

	case data.StepAwaitAnswerEvidence:
		rh.updateCurrentStep(data.StepFailed)
		return ErrAnswerEvidenceTimeout

	case data.StepJudgeAnswers:
		rh.updateCurrentStep(data.StepFailed)
		return ErrAnswerJudgingTimeout

	case data.StepCollectClassificationVotes:
		rh.updateCurrentStep(data.StepFailed)
		return ErrNotEnoughAnswerClassificationVotes

	case data.StepAwaitClassificationCertificate:
		rh.updateCurrentStep(data.StepFailed)
		return ErrAnswerClassificationCertificateTimeout

	case data.StepAwaitProposedSynthesis:
		rh.updateCurrentStep(data.StepFailed)
		return ErrProposedSynthesisTimeout

	case data.StepCollectSynthesisVotes:
		rh.updateCurrentStep(data.StepFailed)
		return ErrSynthesisVotesTimeout

	case data.StepAwaitAggregatedSynthesisVotes:
		rh.updateCurrentStep(data.StepFailed)
		return ErrAggregatedSynthesisVotesTimeout

	default:
		return nil
	}
}

// HandleClassificationGracePeriodElapsed routes the leader's bounded
// post-quorum collection timer through the owning event loop.
func (rh *roundHandler) HandleClassificationGracePeriodElapsed(roundKey data.RoundKey) error {
	if roundKey != rh.currentRoundKey || rh.currentStep != data.StepCollectClassificationVotes {
		rh.logger.Info("ignoring stale classification grace event", "roundKey", roundKey, "currentRoundKey", rh.currentRoundKey, "currentStep", rh.currentStep)
		return nil
	}
	return rh.miniRoundTwoHandler.HandleClassificationGracePeriodElapsed(roundKey)
}

func (rh *roundHandler) handleProposedSynthesis(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepAwaitProposedSynthesis {
		rh.logger.Error("unexpected proposed synthesis message for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	msg := message.ProposedSynthesis
	if msg == nil {
		rh.logger.Error("proposed synthesis message is nil")
		return ErrNilProposedSynthesisMessage
	}

	roundKey := data.RoundKey{Epoch: msg.Epoch, Round: msg.Round, MiniRound: msg.MiniRound}
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("proposed synthesis for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling proposed synthesis", "roundKey", roundKey, "senderID", msg.SenderID)
	if err := rh.miniRoundThreeHandler.HandleProposedSynthesis(roundKey, msg); err != nil {
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("proposed synthesis handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.updateCurrentStep(data.StepAwaitAggregatedSynthesisVotes)
	rh.logger.Info("round step changed", "roundKey", roundKey, "step", rh.currentStep)
	return nil
}

func (rh *roundHandler) handleSynthesisVote(message data.ConsensusMessage) error {
	vote := message.SynthesisVote
	if vote == nil {
		rh.logger.Error("synthesis vote is nil")
		return ErrNilSynthesisVoteMessage
	}

	roundKey := data.RoundKey{Epoch: vote.Epoch, Round: vote.Round, MiniRound: vote.MiniRound}

	if rh.shouldIgnoreFinalizedMiniRoundMessage(roundKey) {
		rh.logger.Info("ignoring synthesis vote for finalized mini-round", "currentRoundKey", rh.currentRoundKey, "messageRoundKey", roundKey)
		return nil
	}

	if rh.currentStep != data.StepCollectSynthesisVotes {
		rh.logger.Error("unexpected synthesis vote for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	if roundKey != rh.currentRoundKey {
		rh.logger.Error("synthesis vote for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling synthesis vote", "roundKey", roundKey, "voterID", vote.VoterID)
	if err := rh.miniRoundThreeHandler.HandleSynthesisVote(roundKey, vote); err != nil {
		return err
	}

	if rh.isFinalizedRoundKey(roundKey) {
		rh.updateCurrentStep(data.StepFinished)
		rh.logger.Info("mini-round three finalized", "roundKey", roundKey)
		if err := rh.finalizeRound(roundKey); err != nil {
			rh.logger.Error("finalizeRound failed", "roundKey", roundKey, "error", err)
			return err
		}
	}

	return nil
}

func (rh *roundHandler) handleAggregatedSynthesisVotes(message data.ConsensusMessage) error {
	if rh.currentStep != data.StepAwaitAggregatedSynthesisVotes {
		rh.logger.Error("unexpected aggregated synthesis votes for step", "currentStep", rh.currentStep)
		return ErrUnexpectedMessageForStep
	}

	msg := message.AggregatedSynthesisVotes
	if msg == nil {
		rh.logger.Error("aggregated synthesis votes message is nil")
		return ErrNilAggregatedSynthesisVotesMessage
	}

	roundKey := data.RoundKey{Epoch: msg.Epoch, Round: msg.Round, MiniRound: msg.MiniRound}
	if roundKey != rh.currentRoundKey {
		rh.logger.Error("aggregated synthesis votes for different round", "expectedRoundKey", rh.currentRoundKey, "actualRoundKey", roundKey)
		return ErrMessageForDifferentRound
	}

	rh.logger.Info("handling aggregated synthesis votes", "roundKey", roundKey, "senderID", msg.SenderID, "numSigners", len(msg.Signers))
	if err := rh.miniRoundThreeHandler.HandleAggregatedSynthesisVotes(roundKey, msg); err != nil {
		rh.updateCurrentStep(data.StepFailed)
		rh.logger.Error("aggregated synthesis votes handling failed", "roundKey", roundKey, "error", err)
		return err
	}

	rh.updateCurrentStep(data.StepFinished)
	rh.logger.Info("mini-round three finalized", "roundKey", roundKey)
	if err := rh.finalizeRound(roundKey); err != nil {
		rh.logger.Error("finalizeRound failed", "roundKey", roundKey, "error", err)
		return err
	}
	return nil
}

// finalizeRound promotes the MR3 block to the canonical chain, cleans up
// per-round state, updates blockchainState, and starts the next full round.
// When chain/mempool/roundState/blockchainState are not wired (e.g. in
// MR1/MR2-only test setups) the function is a no-op.
func (rh *roundHandler) finalizeRound(mr3RoundKey data.RoundKey) error {
	if rh.chain == nil {
		return nil
	}

	block, err := rh.blockFinalizer.GetFinalizedBlockInMRThree(mr3RoundKey)
	if err != nil {
		return err
	}

	if err = rh.chain.Append(block); err != nil {
		return err
	}

	txHashes := make([][]byte, 0, len(block.Body.Transactions))
	for _, tx := range block.Body.Transactions {
		txHashes = append(txHashes, tx.GetTxHash())
	}
	rh.mempool.RemoveTransactions(txHashes)
	if rh.store != nil {
		for _, h := range txHashes {
			rh.store.Remove(h)
		}
	}

	rh.blockchainState.Update(mr3RoundKey.Round, data.MiniRoundThree, mr3RoundKey.Epoch)

	nextRoundKey := data.RoundKey{
		Epoch:     mr3RoundKey.Epoch,
		Round:     mr3RoundKey.Round + 1,
		MiniRound: uint64(data.MiniRoundOne),
	}
	if rh.miniRoundDuration > 0 {
		time.Sleep(rh.miniRoundDuration)
	}
	return rh.StartRound(nextRoundKey)
}

// updateCurrentStep sets the current consensus step and atomically publishes
// a snapshot so that the live-round HTTP endpoint can read it safely from
// another goroutine without acquiring a lock.
func (rh *roundHandler) updateCurrentStep(step data.Step) {
	rh.currentStep = step
	ss := &roundStateSnapshot{roundKey: rh.currentRoundKey, step: step}
	rh.snapshot.Store(ss)
	if rh.onStepChanged != nil {
		rh.onStepChanged(ss.roundKey, ss.step)
	}
}

// liveState returns the latest atomically stored round key and step.
func (rh *roundHandler) liveState() (data.RoundKey, data.Step) {
	if s := rh.snapshot.Load(); s != nil {
		return s.roundKey, s.step
	}
	return rh.currentRoundKey, rh.currentStep
}
