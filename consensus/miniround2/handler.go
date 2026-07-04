package miniround2

import (
	"bytes"
	"log/slog"
	"sort"

	"moa-chain/agent"
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/broadcast"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validators"
)

type miniRoundTwoHandler struct {
	myID string

	blockProcessor     blockprocessing.BlockProcessor
	roundState         state.RoundState
	broadcaster        broadcast.Broadcaster
	signer             signing.MessageSigner
	validatorRegistry  validators.ValidatorRegistry
	blockchainState    state.BlockchainState
	blockFinalizer     blockFinalizer.BlockFinalizer
	answerJudge        agent.AnswerJudge
	judgeModelMetadata string
	logger             *slog.Logger
}

// MiniRoundTwoHandlerArgs defines the structure that encapsulates all the arguments for the handler.
type MiniRoundTwoHandlerArgs struct {
	MyID string

	BlockProcessor     blockprocessing.BlockProcessor
	RoundState         state.RoundState
	Broadcaster        broadcast.Broadcaster
	Signer             signing.MessageSigner
	ValidatorRegistry  validators.ValidatorRegistry
	BlockchainState    state.BlockchainState
	BlockFinalizer     blockFinalizer.BlockFinalizer
	AnswerJudge        agent.AnswerJudge
	JudgeModelMetadata string
	Logger             *slog.Logger
}

// NewMiniRoundTwoHandler creates a new mini-round two handler.
func NewMiniRoundTwoHandler(args MiniRoundTwoHandlerArgs) *miniRoundTwoHandler {
	return &miniRoundTwoHandler{
		myID:               args.MyID,
		blockProcessor:     args.BlockProcessor,
		roundState:         args.RoundState,
		broadcaster:        args.Broadcaster,
		signer:             args.Signer,
		validatorRegistry:  args.ValidatorRegistry,
		blockchainState:    args.BlockchainState,
		blockFinalizer:     args.BlockFinalizer,
		answerJudge:        args.AnswerJudge,
		judgeModelMetadata: args.JudgeModelMetadata,
		logger:             args.Logger,
	}
}

// HandleConsensusSelection handles the consensus selection for the second mini-round,
// taking into consideration the canonical frequency map finalized in the first mini-round.
func (handler *miniRoundTwoHandler) HandleConsensusSelection(key data.RoundKey) (string, error) {
	handler.logger.Info("miniround2.HandleConsensusSelection started", "roundKey", key)

	finalizedBlockInMRO, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch:     key.Epoch,
		Round:     key.Round,
		MiniRound: key.MiniRound - 1,
	})
	if err != nil {
		handler.logger.Error("miniround2.HandleConsensusSelection GetFinalizedBlockInMROne", "err", err)
		return "", err
	}

	err = handler.validatorRegistry.GenerateConsensusGroupMiniRoundTwo(handler.blockchainState, key, finalizedBlockInMRO.SubdomainsFrequencies)
	if err != nil {
		handler.logger.Error("miniround2.HandleConsensusSelection failed", "roundKey", key, "error", err)
		return "", err
	}

	consensusGroup, groupErr := handler.validatorRegistry.ConsensusGroup()
	if groupErr == nil {
		handler.logger.Info("miniround2.HandleConsensusSelection selected consensus group", "roundKey", key, "consensusGroup", consensusGroup)
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.HandleConsensusSelection failed to read leader", "roundKey", key, "error", err)
		return "", err
	}

	handler.logger.Info("miniround2.HandleConsensusSelection selected leader", "roundKey", key, "leaderID", leaderID)
	return leaderID, nil
}

// HandleBlockExecution handles the execution of the prompts (transactions) finalized in the first mini-round.
// This method should be called by each validator implied in the consensus of the second mini-round.
// Each validator should sign its execution of the prompts and send
func (handler *miniRoundTwoHandler) HandleBlockExecution(roundKey data.RoundKey) error {
	handler.logger.Info("miniround2.HandleBlockExecution started", "roundKey", roundKey)

	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		handler.logger.Info("miniround2.HandleBlockExecution local node is not in consensus group; skipping prompt execution", "roundKey", roundKey, "validatorID", handler.myID)
		return nil
	}

	finalizedBlockInMRO, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		handler.logger.Error("miniround2.HandleConsensusSelection GetFinalizedBlockInMROne", "err", err)
		return err
	}

	blockBody := finalizedBlockInMRO.Block.Body
	executionResult, err := handler.blockProcessor.ExecuteBlockPrompts(&blockBody)
	if err != nil {
		handler.logger.Error("miniround2.HandleBlockExecution failed when executing the prompts", "err", err)
		return err
	}

	leader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.HandleBlockExecution failed to read leader", "err", err)
		return err
	}

	executedPromptsMessage, err := handler.createExecutePromptsMessage(roundKey, finalizedBlockInMRO.Block.Header.HeaderHash, executionResult)
	if err != nil {
		handler.logger.Error("miniround2.HandleBlockExecution failed to create execute prompts message", "err", err)
		return err
	}

	signature, err := handler.signer.Sign(executionResult.BlockHash)
	if err != nil {
		handler.logger.Error("miniround2.HandleBlockExecution failed to sign prompt execution hash", "err", err)
		return err
	}
	executedPromptsMessage.BlockSignature = signature

	err = handler.broadcaster.SendVoteToLeader(&data.ConsensusMessage{
		ConsensusMessageType: data.ExecutedPromptsMessage,
		ExecutedPrompts:      executedPromptsMessage,
	}, leader)
	if err != nil {
		handler.logger.Error("miniround2.HandleBlockExecution failed to send vote to leader", "err", err)
		return err
	}

	return nil
}

func (handler *miniRoundTwoHandler) createExecutePromptsMessage(
	roundKey data.RoundKey,
	canonicalBlockHeaderHash []byte,
	executionResult *data.BlockBodyExecutionResultMRTwo,
) (*data.AnswersBlockMessage, error) {
	answers := make(data.AnswersTxMessage)
	for _, txResult := range executionResult.TxsResults {
		txHash := txResult.TxHash

		_, ok := answers[string(txHash)]
		if ok {
			return nil, ErrDuplicatedAnswer
		}

		answers[string(txHash)] = txResult
	}

	return &data.AnswersBlockMessage{
		Epoch:              roundKey.Epoch,
		Round:              roundKey.Round,
		MiniRound:          roundKey.MiniRound,
		SenderID:           handler.myID,
		Answers:            answers,
		CanonicalBlockHash: canonicalBlockHeaderHash,
		BlockHash:          executionResult.BlockHash,
	}, nil
}

// HandleExecutedPromptsMessage handles the event of receiving an execution result.
// The method should be called only by the leader of the consensus group and only the leader should receive.
func (handler *miniRoundTwoHandler) HandleExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error {
	if message == nil {
		return ErrNilAnswers
	}

	handler.logger.Info("miniround2.HandleExecutedPromptsMessage leader received block vote", "roundKey", roundKey, "signerID", message.SenderID)

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage leader failed to get selected leader while handling vote", "roundKey", roundKey, "error", err)
		return err
	}

	if leaderID != handler.myID {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage non-leader attempted to collect votes", "roundKey", roundKey, "expectedLeader", leaderID, "localNode", handler.myID)
		return ErrOnlyLeaderCanCollectVotes
	}

	err = handler.verifyExecutePromptsMessage(roundKey, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to verify executed prompts message", "roundKey", roundKey, "senderID", message.SenderID, "error", err)
		return err
	}

	err = handler.roundState.AddExecutedPromptsMessage(roundKey, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to store executed prompts message", "roundKey", roundKey, "senderID", message.SenderID, "error", err)
		return err
	}

	answers, err := handler.roundState.GetExecutedPromptsMessages(roundKey)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to get executed prompts messages", "roundKey", roundKey, "error", err)
		return err
	}

	consensusGroupSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to get consensus group size", "roundKey", roundKey, "error", err)
		return err
	}

	if uint64(len(answers)) < (2*consensusGroupSize)/3+1 || handler.roundState.IsCertificateSet(roundKey) {
		handler.logger.Info(
			"miniround2.HandleExecutedPromptsMessage leader waiting for more votes or certificate already set",
			"roundKey", roundKey,
			"numNodesWhichSentExecutionResults", len(answers),
			"consensusGroupSize", consensusGroupSize,
			"quorum", (2*consensusGroupSize)/3+1,
			"certificateSet", handler.roundState.IsCertificateSet(roundKey),
		)

		return nil
	}

	// From this point the leader has enough independently verified execution results to build
	// the mini-round two certificate. The certificate is evidence only; the finalized
	// txHash -> answers view is derived locally after the certificate is broadcast.
	handler.logger.Info("miniround2.HandleExecutedPromptsMessage leader reached quorum", "roundKey", roundKey, "numNodesWhichSentExecutionResults", len(answers), "consensusGroupSize", consensusGroupSize)

	aggregatedExecutionResults, err := handler.createAggregatedExecutionResultsMessage(roundKey, answers)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to aggregate execution results", "roundKey", roundKey, "error", err)
		return err
	}

	handler.logger.Info(
		"miniround2.HandleExecutedPromptsMessage leader aggregated execution results",
		"roundKey", roundKey,
		"numSigners", len(aggregatedExecutionResults.Signers),
		"numAnswerSets", len(aggregatedExecutionResults.Answers),
	)

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType:       data.AggregatedExecutionResultsConsensusMessage,
		AggregatedExecutionResults: aggregatedExecutionResults,
	}

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	handler.logger.Info(
		"miniround2.HandleExecutedPromptsMessage leader broadcasting aggregated execution results",
		"roundKey", roundKey,
		"numReceivers", len(validatorsIDs),
	)

	err = handler.broadcaster.BroadcastAggregatedExecutionResults(consensusMessage, handler.myID, validatorsIDs)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage leader failed to broadcast aggregated execution results", "roundKey", roundKey, "error", err)
		return err
	}

	err = handler.finalizeAggregatedExecutionResultsBlock(roundKey, aggregatedExecutionResults)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage leader failed to finalize aggregated execution results block", "roundKey", roundKey, "error", err)
		return err
	}

	handler.logger.Info("miniround2.HandleExecutedPromptsMessage leader finalized aggregated execution results block", "roundKey", roundKey)

	return nil
}

func (handler *miniRoundTwoHandler) finalizeAggregatedExecutionResultsBlock(
	roundKey data.RoundKey,
	aggregatedExecutionResultsMessage *data.AggregatedExecutionResultsMessage,
) error {
	finalizedBlockInMROne, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		return err
	}

	aggregatedExecutionResults, err := handler.createFinalizedAggregatedExecutionResults(aggregatedExecutionResultsMessage)
	if err != nil {
		return err
	}

	// Mini-round two finalization keeps the canonical mini-round one block data and stores only
	// the deterministic execution-result aggregation. The broadcast certificate remains the proof
	// used to recompute this finalized view, but it is not itself the finalized artifact.
	return handler.blockFinalizer.FinalizeBlockMRTwo(roundKey, &data.BlockOnChain{
		Block:                      finalizedBlockInMROne.Block,
		SubdomainsFrequencies:      finalizedBlockInMROne.SubdomainsFrequencies,
		AggregatedExecutionResults: aggregatedExecutionResults,
	})
}

// HandleAggregatedExecutionResults verifies and finalizes the execution result certificate broadcast by the mini-round two leader.
// Each receiver first checks that the message comes from the selected leader and that the certificate arrays are aligned.
// It then reconstructs every validator execution result from the signer, hash, signature, and answer slices, and verifies it
// with the same rules used by the leader: signer membership, canonical block hash, execution result hash, and signature validity.
// After all entries are valid, the receiver locally derives the deterministic txHash -> answers aggregation and finalizes it.
func (handler *miniRoundTwoHandler) HandleAggregatedExecutionResults(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	handler.logger.Info("miniround2.HandleAggregatedExecutionResults received aggregated execution results", "roundKey", roundKey)
	if err := handler.verifyAggregatedExecutionResultsMessage(roundKey, message); err != nil {
		return err
	}

	err := handler.finalizeAggregatedExecutionResultsBlock(roundKey, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleAggregatedExecutionResults failed to finalize aggregated execution results block", "roundKey", roundKey, "error", err)
		return err
	}

	handler.logger.Info("miniround2.HandleAggregatedExecutionResults finalized aggregated execution results block", "roundKey", roundKey, "numSigners", len(message.Signers))
	return nil
}

// verifyAggregatedExecutionResultsMessage validates the leader envelope and
// every signed producer answer without finalizing any block state.
func (handler *miniRoundTwoHandler) verifyAggregatedExecutionResultsMessage(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	if message == nil {
		handler.logger.Error("miniround2.verifyAggregatedExecutionResultsMessage received nil evidence", "roundKey", roundKey)
		return ErrNilAggregatedExecutionResults
	}

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.verifyAggregatedExecutionResultsMessage failed to get leader", "roundKey", roundKey, "error", err)
		return err
	}

	if message.SenderID != expectedLeader {
		handler.logger.Error("miniround2.verifyAggregatedExecutionResultsMessage message not from expected leader", "roundKey", roundKey, "senderID", message.SenderID, "expectedLeader", expectedLeader)
		return ErrMessageNotFromLeader
	}

	if message.Epoch != roundKey.Epoch || message.Round != roundKey.Round || message.MiniRound != roundKey.MiniRound {
		handler.logger.Error("miniround2.verifyAggregatedExecutionResultsMessage round key mismatch", "roundKey", roundKey, "messageEpoch", message.Epoch, "messageRound", message.Round, "messageMiniRound", message.MiniRound)
		return ErrAggregatedExecutionResultsMismatch
	}

	if len(message.Signers) != len(message.BlockHashes) ||
		len(message.Signers) != len(message.BlockSignatures) ||
		len(message.Signers) != len(message.Answers) {
		handler.logger.Error(
			"miniround2.verifyAggregatedExecutionResultsMessage inconsistent certificate array lengths",
			"roundKey", roundKey,
			"numSigners", len(message.Signers),
			"numBlockHashes", len(message.BlockHashes),
			"numBlockSignatures", len(message.BlockSignatures),
			"numAnswerSets", len(message.Answers),
		)
		return ErrAggregatedExecutionResultsMismatch
	}

	for index, signerID := range message.Signers {
		executedPromptsMessage := &data.AnswersBlockMessage{
			Epoch:              message.Epoch,
			Round:              message.Round,
			MiniRound:          message.MiniRound,
			SenderID:           signerID,
			Answers:            message.Answers[index],
			CanonicalBlockHash: message.CanonicalBlockHash,
			BlockHash:          message.BlockHashes[index],
			BlockSignature:     message.BlockSignatures[index],
		}
		if err = handler.verifyExecutePromptsMessage(roundKey, executedPromptsMessage); err != nil {
			handler.logger.Error("miniround2.verifyAggregatedExecutionResultsMessage failed to verify execution result", "roundKey", roundKey, "index", index, "signerID", signerID, "error", err)
			return err
		}
	}

	return nil
}

// HandleAnswerEvidenceForClassification verifies the leader's complete answer
// evidence before invoking the local judge and producing a signed vote. This
// method is intentionally not called by the active round flow until activation.
func (handler *miniRoundTwoHandler) HandleAnswerEvidenceForClassification(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	if err := handler.verifyAggregatedExecutionResultsMessage(roundKey, message); err != nil {
		return err
	}
	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		handler.logger.Error("miniround2.HandleAnswerEvidenceForClassification local judge is outside consensus group", "roundKey", roundKey, "judgeID", handler.myID)
		return ErrValidatorNotPartOfConsensusGroup
	}

	finalizedBlock, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidenceForClassification failed to load canonical block", "roundKey", roundKey, "error", err)
		return err
	}

	requests, err := BuildAnswerJudgeRequests(&finalizedBlock.Block, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidenceForClassification failed to build judge requests", "roundKey", roundKey, "error", err)
		return err
	}

	assignments, err := JudgeAnswerRequests(handler.answerJudge, requests)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidenceForClassification judge failed", "roundKey", roundKey, "judgeID", handler.myID, "error", err)
		return err
	}

	vote, err := handler.createSignedAnswerClassificationVote(roundKey, message, requests, assignments)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidenceForClassification failed to create vote", "roundKey", roundKey, "judgeID", handler.myID, "error", err)
		return err
	}

	return handler.dispatchAnswerClassificationVote(roundKey, vote)
}

// createSignedAnswerClassificationVote validates complete candidate coverage,
// binds assignments to the exact evidence and prompt, and signs the vote.
func (handler *miniRoundTwoHandler) createSignedAnswerClassificationVote(
	roundKey data.RoundKey,
	evidence *data.AggregatedExecutionResultsMessage,
	requests []TransactionAnswerJudgeRequest,
	assignments []data.AnswerClassificationAssignment,
) (*data.AnswerClassificationVote, error) {
	expectedCandidates := judgeRequestCandidateIDs(requests)
	if err := ValidateClassificationAssignments(expectedCandidates, assignments); err != nil {
		return nil, err
	}
	evidenceHash, err := hashing.ComputeAnswerEvidenceHash(evidence)
	if err != nil {
		return nil, err
	}

	vote := &data.AnswerClassificationVote{
		Epoch:              roundKey.Epoch,
		Round:              roundKey.Round,
		MiniRound:          roundKey.MiniRound,
		CanonicalBlockHash: evidence.CanonicalBlockHash,
		AnswerEvidenceHash: evidenceHash,
		JudgeID:            handler.myID,
		PromptVersion:      AnswerJudgePromptVersion,
		PromptHash:         AnswerJudgePromptHash(),
		ModelMetadata:      handler.judgeModelMetadata,
		Assignments:        assignments,
	}
	if err = handler.signAnswerClassificationVote(vote, evidenceHash); err != nil {
		return nil, err
	}

	return vote, nil
}

// judgeRequestCandidateIDs extracts the protocol identities hidden behind all
// per-transaction model aliases.
func judgeRequestCandidateIDs(requests []TransactionAnswerJudgeRequest) []data.AnswerCandidateID {
	candidates := make([]data.AnswerCandidateID, 0)
	for _, request := range requests {
		for _, reference := range request.Candidates {
			candidates = append(candidates, reference.CandidateID)
		}
	}

	return CanonicalizeAnswerCandidateIDs(candidates)
}

// signAnswerClassificationVote rejects unexpected evidence or prompt identity
// before invoking the node's signer.
func (handler *miniRoundTwoHandler) signAnswerClassificationVote(
	vote *data.AnswerClassificationVote,
	expectedEvidenceHash []byte,
) error {
	if !bytes.Equal(vote.AnswerEvidenceHash, expectedEvidenceHash) {
		return ErrClassificationVoteEvidenceMismatch
	}
	if vote.PromptVersion != AnswerJudgePromptVersion ||
		!bytes.Equal(vote.PromptHash, AnswerJudgePromptHash()) {
		return ErrClassificationVotePromptMismatch
	}

	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	if err != nil {
		return err
	}

	signature, err := handler.signer.Sign(voteHash)
	if err != nil {
		return err
	}

	vote.VoteHash = voteHash
	vote.Signature = signature

	return nil
}

// dispatchAnswerClassificationVote sends validator votes to the leader and
// processes the leader's own vote through the same storage handler.
func (handler *miniRoundTwoHandler) dispatchAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) error {
	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if leaderID == handler.myID {
		return handler.HandleAnswerClassificationVote(roundKey, vote)
	}

	err = handler.broadcaster.SendAnswerClassificationVoteToLeader(&data.ConsensusMessage{
		ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
		AnswerClassificationVote: vote,
	}, leaderID)
	if err != nil {
		handler.logger.Error("miniround2.dispatchAnswerClassificationVote failed to send vote", "roundKey", roundKey, "judgeID", handler.myID, "leaderID", leaderID, "error", err)
	}

	return err
}

// HandleAnswerClassificationVote verifies and stores one judge vote. Once the
// leader reaches classification quorum, it derives and broadcasts the canonical
// classification certificate.
func (handler *miniRoundTwoHandler) HandleAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) error {
	if err := handler.verifyClassificationCollectionLeader(roundKey); err != nil {
		return err
	}

	expectedCandidates, err := handler.verifyAnswerClassificationVote(roundKey, vote)
	if err != nil {
		judgeID := ""
		if vote != nil {
			judgeID = vote.JudgeID
		}
		handler.logger.Error("miniround2.HandleAnswerClassificationVote rejected vote", "roundKey", roundKey, "judgeID", judgeID, "error", err)
		return err
	}
	if handler.roundState.IsAnswerClassificationCertificateSet(roundKey) {
		handler.logger.Info("miniround2.HandleAnswerClassificationVote certificate already produced", "roundKey", roundKey, "judgeID", vote.JudgeID)
		return nil
	}
	if err = handler.roundState.AddAnswerClassificationVote(roundKey, vote); err != nil {
		handler.logger.Error("miniround2.HandleAnswerClassificationVote failed to store vote", "roundKey", roundKey, "judgeID", vote.JudgeID, "error", err)
		return err
	}

	return handler.broadcastClassificationCertificateAtQuorum(roundKey, expectedCandidates)
}

// verifyClassificationCollectionLeader prevents non-leaders from accepting or
// aggregating classification votes.
func (handler *miniRoundTwoHandler) verifyClassificationCollectionLeader(roundKey data.RoundKey) error {
	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if leaderID != handler.myID {
		handler.logger.Error("miniround2.verifyClassificationCollectionLeader local node is not leader", "roundKey", roundKey, "validatorID", handler.myID, "leaderID", leaderID)
		return ErrOnlyLeaderCanCollectVotes
	}

	return nil
}

// verifyAnswerClassificationVote authenticates a judge vote and checks that it
// covers the exact evidence, prompt, and candidates classified by the leader.
func (handler *miniRoundTwoHandler) verifyAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) ([]data.AnswerCandidateID, error) {
	if vote == nil {
		return nil, ErrNilAnswerClassificationVote
	}
	if !classificationVoteMatchesRound(roundKey, vote) {
		return nil, ErrAnswerClassificationVoteMismatch
	}
	if !handler.validatorRegistry.IsValidatorInConsensusGroup(vote.JudgeID) {
		return nil, ErrValidatorNotPartOfConsensusGroup
	}
	if vote.PromptVersion != AnswerJudgePromptVersion || !bytes.Equal(vote.PromptHash, AnswerJudgePromptHash()) {
		return nil, ErrClassificationVotePromptMismatch
	}

	expectedCandidates, err := handler.classificationCollectionCandidates(roundKey, vote)
	if err != nil {
		return nil, err
	}
	if err = ValidateClassificationAssignments(expectedCandidates, vote.Assignments); err != nil {
		return nil, err
	}
	if err = handler.verifyAnswerClassificationVoteSignature(vote); err != nil {
		return nil, err
	}

	return expectedCandidates, nil
}

// classificationCollectionCandidates uses the leader's signed local vote as
// the trusted context established from verified answer evidence.
func (handler *miniRoundTwoHandler) classificationCollectionCandidates(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) ([]data.AnswerCandidateID, error) {
	votes, err := handler.roundState.GetAnswerClassificationVotes(roundKey)
	if err != nil {
		// The leader's own vote is the first vote and establishes collection context.
		if vote.JudgeID != handler.myID {
			return nil, ErrMissingClassificationCollectionContext
		}

		return assignmentCandidateIDs(vote.Assignments), nil
	}

	for _, storedVote := range votes {
		if storedVote.JudgeID != handler.myID {
			continue
		}
		if !sameClassificationContext(*storedVote, *vote) {
			return nil, ErrClassificationVoteContextMismatch
		}

		return assignmentCandidateIDs(storedVote.Assignments), nil
	}

	return nil, ErrMissingClassificationCollectionContext
}

// assignmentCandidateIDs extracts candidate identities without trusting the
// categories selected by a judge.
func assignmentCandidateIDs(assignments []data.AnswerClassificationAssignment) []data.AnswerCandidateID {
	candidates := make([]data.AnswerCandidateID, len(assignments))
	for index := range assignments {
		candidates[index] = assignments[index].CandidateID
	}

	return CanonicalizeAnswerCandidateIDs(candidates)
}

// verifyAnswerClassificationVoteSignature recomputes the signed payload hash
// and verifies it against the judge's registered public key.
func (handler *miniRoundTwoHandler) verifyAnswerClassificationVoteSignature(
	vote *data.AnswerClassificationVote,
) error {
	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	if err != nil {
		return err
	}
	if !bytes.Equal(voteHash, vote.VoteHash) {
		return ErrClassificationVoteHashMismatch
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(vote.JudgeID)
	if err != nil {
		return err
	}

	return handler.signer.Verify(publicKey, voteHash, vote.Signature)
}

// broadcastClassificationCertificateAtQuorum waits for the configured quorum,
// derives canonical groups, and broadcasts one certificate in judge-ID order.
func (handler *miniRoundTwoHandler) broadcastClassificationCertificateAtQuorum(
	roundKey data.RoundKey,
	expectedCandidates []data.AnswerCandidateID,
) error {
	votePointers, err := handler.roundState.GetAnswerClassificationVotes(roundKey)
	if err != nil {
		return err
	}
	committeeSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}
	quorum := ClassificationQuorum(committeeSize)
	if uint64(len(votePointers)) < quorum {
		handler.logger.Info("miniround2.broadcastClassificationCertificateAtQuorum waiting for votes", "roundKey", roundKey, "numVotes", len(votePointers), "quorum", quorum)
		return nil
	}

	votes := make([]data.AnswerClassificationVote, len(votePointers))
	for index, storedVote := range votePointers {
		votes[index] = *storedVote
	}
	transactions, err := AggregateClassificationVotes(expectedCandidates, votes, committeeSize)
	if err != nil {
		return err
	}
	context := votes[0]
	certificate := &data.AnswerClassificationCertificate{
		Epoch: context.Epoch, Round: context.Round, MiniRound: context.MiniRound,
		SenderID:           handler.myID,
		CanonicalBlockHash: context.CanonicalBlockHash,
		AnswerEvidenceHash: context.AnswerEvidenceHash,
		PromptVersion:      context.PromptVersion,
		PromptHash:         context.PromptHash,
		Votes:              CanonicalizeClassificationVotes(votes),
		Transactions:       transactions,
	}

	consensusGroup, err := handler.validatorRegistry.ConsensusGroup()
	if err != nil {
		return err
	}
	err = handler.broadcaster.BroadcastAnswerClassificationCertificate(&data.ConsensusMessage{
		ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
		AnswerClassificationCertificate: certificate,
	}, handler.myID, consensusGroup)
	if err != nil {
		handler.logger.Error("miniround2.broadcastClassificationCertificateAtQuorum failed to broadcast certificate", "roundKey", roundKey, "error", err)
		return err
	}

	return handler.roundState.SetAnswerClassificationCertificate(roundKey, certificate)
}

// HandleAnswerClassificationCertificate performs structural alignment checks
// and stores the leader certificate without activating finalization.
func (handler *miniRoundTwoHandler) HandleAnswerClassificationCertificate(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) error {
	if certificate == nil {
		handler.logger.Error("miniround2.HandleAnswerClassificationCertificate received nil certificate", "roundKey", roundKey)
		return ErrNilAnswerClassificationCertificate
	}
	if !classificationCertificateMatchesRound(roundKey, certificate) {
		handler.logger.Error(
			"miniround2.HandleAnswerClassificationCertificate certificate envelope mismatch",
			"roundKey", roundKey,
			"senderID", certificate.SenderID,
			"answerEvidenceHash", certificate.AnswerEvidenceHash,
			"promptVersion", certificate.PromptVersion,
		)
		return ErrAnswerClassificationCertificateMismatch
	}

	handler.logger.Info(
		"miniround2.HandleAnswerClassificationCertificate storing certificate",
		"roundKey", roundKey,
		"senderID", certificate.SenderID,
		"answerEvidenceHash", certificate.AnswerEvidenceHash,
		"promptVersion", certificate.PromptVersion,
		"numVotes", len(certificate.Votes),
	)

	err := handler.roundState.SetAnswerClassificationCertificate(roundKey, certificate)
	if err != nil {
		handler.logger.Error(
			"miniround2.HandleAnswerClassificationCertificate failed to store certificate",
			"roundKey", roundKey,
			"senderID", certificate.SenderID,
			"error", err,
		)
	}

	return err
}

// classificationVoteMatchesRound validates fields shared by every classification vote envelope.
func classificationVoteMatchesRound(roundKey data.RoundKey, vote *data.AnswerClassificationVote) bool {
	return vote.Epoch == roundKey.Epoch &&
		vote.Round == roundKey.Round &&
		vote.MiniRound == roundKey.MiniRound &&
		vote.JudgeID != "" &&
		len(vote.CanonicalBlockHash) > 0 &&
		len(vote.AnswerEvidenceHash) > 0 &&
		vote.PromptVersion != "" &&
		len(vote.PromptHash) > 0 &&
		len(vote.Assignments) > 0
}

// classificationCertificateMatchesRound verifies that every embedded vote is
// aligned with the certificate's round, answer evidence, and prompt identity.
func classificationCertificateMatchesRound(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) bool {
	if certificate.Epoch != roundKey.Epoch ||
		certificate.Round != roundKey.Round ||
		certificate.MiniRound != roundKey.MiniRound ||
		certificate.SenderID == "" ||
		len(certificate.CanonicalBlockHash) == 0 ||
		len(certificate.AnswerEvidenceHash) == 0 ||
		certificate.PromptVersion == "" ||
		len(certificate.PromptHash) == 0 ||
		len(certificate.Votes) == 0 ||
		len(certificate.Transactions) == 0 {
		return false
	}

	for index := range certificate.Votes {
		vote := &certificate.Votes[index]
		if !classificationVoteMatchesRound(roundKey, vote) ||
			!bytes.Equal(vote.CanonicalBlockHash, certificate.CanonicalBlockHash) ||
			!bytes.Equal(vote.AnswerEvidenceHash, certificate.AnswerEvidenceHash) ||
			vote.PromptVersion != certificate.PromptVersion ||
			!bytes.Equal(vote.PromptHash, certificate.PromptHash) {
			return false
		}
	}

	return true
}

func (handler *miniRoundTwoHandler) createFinalizedAggregatedExecutionResults(
	message *data.AggregatedExecutionResultsMessage,
) (data.AggregatedExecutionResults, error) {
	if message == nil || len(message.Answers) == 0 {
		return nil, ErrNilAnswers
	}

	txHashSet := make(map[string]struct{})
	for txHash := range message.Answers[0] {
		txHashSet[txHash] = struct{}{}
	}

	// Maps are intentionally converted to a sorted slice before finalization. This makes the
	// finalized txHash -> answers artifact independent from Go map iteration order.
	txHashes := make([]string, 0, len(txHashSet))
	for txHash := range txHashSet {
		txHashes = append(txHashes, txHash)
	}
	sort.Strings(txHashes)

	aggregatedExecutionResults := make(data.AggregatedExecutionResults, 0, len(txHashes))
	for _, txHash := range txHashes {
		answers := make([]data.TransactionResult, 0, len(message.Answers))
		for _, answerSet := range message.Answers {
			// All validators must have answered the same canonical transaction set; otherwise
			// different nodes could derive different finalized aggregates from the same evidence.
			if len(answerSet) != len(txHashSet) {
				return nil, ErrExecutedPromptsAnswersMismatch
			}

			answer, ok := answerSet[txHash]
			if !ok {
				return nil, ErrExecutedPromptsAnswersMismatch
			}
			if !bytes.Equal(answer.TxHash, []byte(txHash)) {
				return nil, ErrExecutedPromptsAnswersMismatch
			}

			answers = append(answers, answer)
		}

		aggregatedExecutionResults = append(aggregatedExecutionResults, data.AggregatedTransactionExecutionResults{
			TxHash:  []byte(txHash),
			Answers: answers,
		})
	}

	return aggregatedExecutionResults, nil
}

func (handler *miniRoundTwoHandler) createAggregatedExecutionResultsMessage(
	roundKey data.RoundKey,
	messages []*data.AnswersBlockMessage,
) (*data.AggregatedExecutionResultsMessage, error) {
	if len(messages) == 0 {
		return nil, ErrNilAnswers
	}

	orderedMessages := make([]*data.AnswersBlockMessage, len(messages))
	copy(orderedMessages, messages)

	// Arrival order is not consensus-safe, so the leader canonicalizes certificate order before
	// broadcasting. Receivers use this order when deriving the finalized answer slices.
	sort.Slice(orderedMessages, func(i, j int) bool {
		if orderedMessages[i] == nil {
			return false
		}
		if orderedMessages[j] == nil {
			return true
		}
		if orderedMessages[i].SenderID == orderedMessages[j].SenderID {
			return bytes.Compare(orderedMessages[i].BlockHash, orderedMessages[j].BlockHash) < 0
		}

		return orderedMessages[i].SenderID < orderedMessages[j].SenderID
	})
	if orderedMessages[0] == nil {
		return nil, ErrNilAnswers
	}

	canonicalBlockHash := orderedMessages[0].CanonicalBlockHash
	signers := make([]string, 0, len(orderedMessages))
	blockHashes := make([][]byte, 0, len(orderedMessages))
	blockSignatures := make([][]byte, 0, len(orderedMessages))
	answers := make([]data.AnswersTxMessage, 0, len(orderedMessages))

	for _, message := range orderedMessages {
		if message == nil {
			return nil, ErrNilAnswers
		}
		if !bytes.Equal(message.CanonicalBlockHash, canonicalBlockHash) {
			return nil, ErrCanonicalBlockHashMismatch
		}

		signers = append(signers, message.SenderID)
		blockHashes = append(blockHashes, message.BlockHash)
		blockSignatures = append(blockSignatures, message.BlockSignature)
		answers = append(answers, message.Answers)
	}

	return &data.AggregatedExecutionResultsMessage{
		Epoch:              roundKey.Epoch,
		Round:              roundKey.Round,
		MiniRound:          roundKey.MiniRound,
		SenderID:           handler.myID,
		CanonicalBlockHash: canonicalBlockHash,
		Signers:            signers,
		BlockHashes:        blockHashes,
		BlockSignatures:    blockSignatures,
		Answers:            answers,
	}, nil
}

func (handler *miniRoundTwoHandler) verifyExecutePromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error {
	signature := message.BlockSignature
	validatorID := message.SenderID

	isValidator := handler.validatorRegistry.IsValidatorRegistered(validatorID)
	if !isValidator {
		handler.logger.Error("executed prompts signer is not registered", "roundKey", roundKey, "signerID", validatorID)
		return ErrSignerIsNotValidator
	}

	isValidatorInConsensusGroup := handler.validatorRegistry.IsValidatorInConsensusGroup(validatorID)
	if !isValidatorInConsensusGroup {
		handler.logger.Error("executed prompts signer is not in consensus group", "roundKey", roundKey, "signerID", validatorID)
		return ErrValidatorNotPartOfConsensusGroup
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(validatorID)
	if err != nil {
		handler.logger.Error("failed to get executed prompts signer public key", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	executionResultHash, err := handler.computeExecutionResultHashFromMessage(roundKey, message)
	if err != nil {
		handler.logger.Error("failed to compute executed prompts hash from message", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	if !bytes.Equal(executionResultHash, message.BlockHash) {
		handler.logger.Error("executed prompts hash mismatch", "roundKey", roundKey, "signerID", validatorID)
		return ErrExecutionResultHashMismatch
	}

	err = handler.signer.Verify(publicKey, executionResultHash, signature)
	if err != nil {
		handler.logger.Error("executed prompts signature verification failed", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	return nil
}

func (handler *miniRoundTwoHandler) computeExecutionResultHashFromMessage(
	roundKey data.RoundKey,
	message *data.AnswersBlockMessage,
) ([]byte, error) {
	finalizedBlockInMROne, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		return nil, err
	}

	canonicalBlock := finalizedBlockInMROne.Block
	if !bytes.Equal(message.CanonicalBlockHash, canonicalBlock.Header.HeaderHash) {
		return nil, ErrCanonicalBlockHashMismatch
	}

	canonicalTxs := canonicalBlock.Body.Transactions
	if len(message.Answers) != len(canonicalTxs) {
		return nil, ErrExecutedPromptsAnswersMismatch
	}

	txResults := make([]data.TransactionResult, 0, len(canonicalTxs))
	totalConsumption := uint64(0)

	// Rebuild the execution result in canonical mini-round one transaction order before hashing.
	// The incoming answers are a map, so hashing them directly would be nondeterministic.
	for _, tx := range canonicalTxs {
		txHash := tx.GetTxHash()

		txResult, ok := message.Answers[string(txHash)]
		if !ok {
			return nil, ErrExecutedPromptsAnswersMismatch
		}

		if !bytes.Equal(txResult.TxHash, txHash) {
			return nil, ErrExecutedPromptsAnswersMismatch
		}

		txResults = append(txResults, txResult)
		totalConsumption += txResult.ActualConsumption
	}

	return hashing.ComputePromptExecutionHash(&data.BlockBodyExecutionResultMRTwo{
		TxsResults:       txResults,
		TotalConsumption: totalConsumption,
	})
}
