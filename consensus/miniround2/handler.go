package miniround2

import (
	"log/slog"

	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/broadcast"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validators"
)

type miniRoundTwoHandler struct {
	myID string

	blockProcessor    blockprocessing.BlockProcessor
	roundState        state.RoundState
	broadcaster       broadcast.Broadcaster
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
	blockchainState   state.BlockchainState
	blockFinalizer    blockFinalizer.BlockFinalizer
	logger            *slog.Logger
}

// MiniRoundTwoHandlerArgs defines the structure that encapsulates all the arguments for the handler.
type MiniRoundTwoHandlerArgs struct {
	MyID string

	BlockProcessor    blockprocessing.BlockProcessor
	RoundState        state.RoundState
	Broadcaster       broadcast.Broadcaster
	Signer            signing.MessageSigner
	ValidatorRegistry validators.ValidatorRegistry
	BlockchainState   state.BlockchainState
	BlockFinalizer    blockFinalizer.BlockFinalizer
	Logger            *slog.Logger
}

// NewMiniRoundTwoHandler creates a new mini-round two handler.
func NewMiniRoundTwoHandler(args MiniRoundTwoHandlerArgs) *miniRoundTwoHandler {
	return &miniRoundTwoHandler{
		myID:              args.MyID,
		blockProcessor:    args.BlockProcessor,
		roundState:        args.RoundState,
		broadcaster:       args.Broadcaster,
		signer:            args.Signer,
		validatorRegistry: args.ValidatorRegistry,
		blockFinalizer:    args.BlockFinalizer,
		logger:            args.Logger,
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

	signature, err := handler.signer.SignPromptExecutionHash(executionResult.BlockHash)
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

func (handler *miniRoundTwoHandler) HandleExecutePromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error {
	if message == nil {
		return ErrNilAnswers
	}

	handler.logger.Info("miniround2.HandleExecutePromptsMessage leader received block vote", "roundKey", roundKey, "signerID", message.SenderID)

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutePromptsMessage leader failed to get selected leader while handling vote", "roundKey", roundKey, "error", err)
		return err
	}

	if leaderID != handler.myID {
		handler.logger.Error("miniround2.HandleExecutePromptsMessage non-leader attempted to collect votes", "roundKey", roundKey, "expectedLeader", leaderID, "localNode", handler.myID)
		return ErrOnlyLeaderCanCollectVotes
	}

	err = handler.verifyExecutePromptsMessage(roundKey, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleExecutePromptsMessage failed to verify executed prompts message", "roundKey", roundKey, "senderID", message.SenderID, "error", err)
		return err
	}

	return nil
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

	err = handler.signer.Verify(publicKey, message.BlockHash, signature)
	if err != nil {
		handler.logger.Error("executed prompts signature verification failed", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	return nil
}
