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

// miniRoundTwoHandler owns network-facing mini-round-two orchestration. Pure
// judging and aggregation rules live in the classification subpackage so this
// type only coordinates verified messages, round state, and finalization.
type miniRoundTwoHandler struct {
	myID string

	blockProcessor     blockprocessing.BlockProcessor
	roundState         state.RoundState
	broadcaster        broadcast.Broadcaster
	signer             signing.MessageSigner
	validatorRegistry  validators.ValidatorRegistry
	blockchainState    state.BlockchainState
	blockFinalizer     blockFinalizer.BlockFinalizer
	answerJudge        agent.AnswersJudge
	judgeModelMetadata string
	logger             *slog.Logger
}

// MiniRoundTwoHandlerArgs contains the node-local dependencies used by both the
// answer-evidence and classification phases.
type MiniRoundTwoHandlerArgs struct {
	MyID string

	BlockProcessor     blockprocessing.BlockProcessor
	RoundState         state.RoundState
	Broadcaster        broadcast.Broadcaster
	Signer             signing.MessageSigner
	ValidatorRegistry  validators.ValidatorRegistry
	BlockchainState    state.BlockchainState
	BlockFinalizer     blockFinalizer.BlockFinalizer
	AnswerJudge        agent.AnswersJudge
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

// HandleBlockExecution executes the canonical mini-round-one transactions on a
// selected producer, signs the resulting commitment, and sends it to the
// mini-round-two leader. Nodes outside the committee intentionally do nothing.
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

// createExecutePromptsMessage converts execution results into the map carried
// over the network while rejecting more than one result for the same tx hash.
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
	// TODO: Define whether any valid execution-result quorum must lead to the
	// same mini-round-two transaction eligibility decisions, or whether the
	// protocol explicitly accepts leader/network timing as choosing the evidence
	// quorum. If eligibility must be invariant, replace first-quorum finalization
	// with a canonical or threshold-stable evidence selection rule.
	handler.logger.Info("miniround2.HandleExecutedPromptsMessage leader reached quorum", "roundKey", roundKey, "numNodesWhichSentExecutionResults", len(answers), "consensusGroupSize", consensusGroupSize)

	answerEvidence, err := handler.createAnswerEvidence(roundKey, answers)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage failed to aggregate execution results", "roundKey", roundKey, "error", err)
		return err
	}

	handler.logger.Info(
		"miniround2.HandleExecutedPromptsMessage leader built answer evidence",
		"roundKey", roundKey,
		"numSigners", len(answerEvidence.Signers),
		"numAnswerSets", len(answerEvidence.Answers),
	)

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.AnswerEvidenceConsensusMessage,
		AnswerEvidence:       answerEvidence,
	}

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	handler.logger.Info(
		"miniround2.HandleExecutedPromptsMessage leader broadcasting answer evidence",
		"roundKey", roundKey,
		"numReceivers", len(validatorsIDs),
	)

	err = handler.broadcaster.BroadcastAnswerEvidence(consensusMessage, handler.myID, validatorsIDs)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutedPromptsMessage leader failed to broadcast answer evidence", "roundKey", roundKey, "error", err)
		return err
	}

	// The evidence broadcast no longer finalizes mini-round two. The leader judges
	// the same verified evidence locally and enters classification vote collection.
	return handler.HandleAnswerEvidence(roundKey, answerEvidence)
}

// verifyAnswerEvidence validates the leader envelope and
// every signed producer answer without finalizing any block state. The message
// uses parallel slices, so their alignment is checked before indexing them.
func (handler *miniRoundTwoHandler) verifyAnswerEvidence(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	if message == nil {
		handler.logger.Error("miniround2.verifyAnswerEvidence received nil evidence", "roundKey", roundKey)
		return ErrNilAnswerEvidence
	}

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.verifyAnswerEvidence failed to get leader", "roundKey", roundKey, "error", err)
		return err
	}

	if message.SenderID != expectedLeader {
		handler.logger.Error("miniround2.verifyAnswerEvidence message not from expected leader", "roundKey", roundKey, "senderID", message.SenderID, "expectedLeader", expectedLeader)
		return ErrMessageNotFromLeader
	}

	if message.Epoch != roundKey.Epoch || message.Round != roundKey.Round || message.MiniRound != roundKey.MiniRound {
		handler.logger.Error("miniround2.verifyAnswerEvidence round key mismatch", "roundKey", roundKey, "messageEpoch", message.Epoch, "messageRound", message.Round, "messageMiniRound", message.MiniRound)
		return ErrAnswerEvidenceMismatch
	}

	if len(message.Signers) != len(message.BlockHashes) ||
		len(message.Signers) != len(message.BlockSignatures) ||
		len(message.Signers) != len(message.Answers) {
		handler.logger.Error(
			"miniround2.verifyAnswerEvidence inconsistent certificate array lengths",
			"roundKey", roundKey,
			"numSigners", len(message.Signers),
			"numBlockHashes", len(message.BlockHashes),
			"numBlockSignatures", len(message.BlockSignatures),
			"numAnswerSets", len(message.Answers),
		)
		return ErrAnswerEvidenceMismatch
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
			handler.logger.Error("miniround2.verifyAnswerEvidence failed to verify execution result", "roundKey", roundKey, "index", index, "signerID", signerID, "error", err)
			return err
		}
	}

	return nil
}

// createAnswerEvidence creates the answer-evidence
// certificate. Producer messages are sorted because network arrival order must
// never influence a consensus-visible certificate.
func (handler *miniRoundTwoHandler) createAnswerEvidence(
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

// verifyExecutePromptsMessage reconstructs the producer's signed commitment
// from canonical mini-round-one transaction order before checking its signature.
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

// computeExecutionResultHashFromMessage converts the answer map back into
// canonical transaction order; hashing a Go map directly would be nondeterministic.
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
