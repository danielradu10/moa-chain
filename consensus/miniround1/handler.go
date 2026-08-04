package miniround1

import (
	"fmt"
	"log/slog"

	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/broadcast"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/state"
	"moa-chain/validators"
)

type handler struct {
	myID string

	blockCreator      blockprocessing.BlockCreator
	blockValidator    blockprocessing.BlockProcessor
	labelsValidator   blockprocessing.LabelsValidator
	roundState        state.RoundState
	broadcaster       broadcast.Broadcaster
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
	blockchainState   state.BlockchainState
	blockFinalizer    blockFinalizer.BlockFinalizer
	logger            *slog.Logger
}

type MiniRoundOneHandlerArgs struct {
	MyID string

	BlockCreator      blockprocessing.BlockCreator
	BlockProcessor    blockprocessing.BlockProcessor
	LabelsValidator   blockprocessing.LabelsValidator
	RoundState        state.RoundState
	Broadcaster       broadcast.Broadcaster
	Signer            signing.MessageSigner
	ValidatorRegistry validators.ValidatorRegistry
	BlockchainState   state.BlockchainState
	BlockFinalizer    blockFinalizer.BlockFinalizer
	Logger            *slog.Logger
}

// NewMiniRoundOneHandler returns a new mini round one handler
func NewMiniRoundOneHandler(args MiniRoundOneHandlerArgs) *handler {
	return &handler{
		myID:              args.MyID,
		blockCreator:      args.BlockCreator,
		blockValidator:    args.BlockProcessor,
		labelsValidator:   args.LabelsValidator,
		roundState:        args.RoundState,
		broadcaster:       args.Broadcaster,
		signer:            args.Signer,
		validatorRegistry: args.ValidatorRegistry,
		blockchainState:   args.BlockchainState,
		blockFinalizer:    args.BlockFinalizer,
		logger:            logging.FromOptional(args.Logger),
	}
}

// HandleConsensusSelection should be called by each validator in the beginning of the round.
func (handler *handler) HandleConsensusSelection(key data.RoundKey) (string, error) {
	handler.logger.Info("miniround1.HandleConsensusSelection started", "roundKey", key)

	err := handler.validatorRegistry.GenerateConsensusGroupMiniRoundOne(handler.blockchainState, key)
	if err != nil {
		handler.logger.Error("miniround1.HandleConsensusSelection failed", "roundKey", key, "error", err)
		return "", err
	}

	consensusGroup, groupErr := handler.validatorRegistry.ConsensusGroup()
	if groupErr == nil {
		handler.logger.Info("miniround1.HandleConsensusSelection selected consensus group", "roundKey", key, "consensusGroup", consensusGroup)
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround1.HandleConsensusSelection failed to read leader", "roundKey", key, "error", err)
		return "", err
	}

	handler.logger.Info("miniround1.HandleConsensusSelection selected leader", "roundKey", key, "leaderID", leaderID)
	return leaderID, nil
}

// HandleProposingBlock handles proposing a block.
// This method will be called when a validator becomes leader in a specific mini-round.
func (handler *handler) HandleProposingBlock(roundKey data.RoundKey) error {
	handler.logger.Info("miniround1.HandleProposingBlock leader started block proposal", "roundKey", roundKey)

	// TODO should propose the domains also?
	block, subdomains, subdomainsHash, err := handler.blockCreator.ProposeBlockAndDomains()
	if err != nil {
		handler.logger.Error("miniround1.HandleProposingBlock failed to propose block and domains", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Info(
		"miniround1.HandleProposingBlock proposed block and domains",
		"roundKey", roundKey,
		"numTransactions", len(block.Body.Transactions),
		"numSubdomainMaps", len(subdomains),
		"blockHashLen", len(block.Header.HeaderHash),
		"subdomainsHashLen", len(subdomainsHash),
	)

	// create the blockSignature of the subdomains
	subdomainsSignature, err := handler.signer.Sign(subdomainsHash)
	if err != nil {
		handler.logger.Error("leader failed to sign subdomains", "roundKey", roundKey, "error", err)
		return err
	}

	// there is no need to process the block, we already processed it inside ProposeBlockAndDomains

	err = handler.roundState.SetProposedBlock(roundKey, block)
	if err != nil {
		handler.logger.Error("leader failed to save proposed block in round state", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Debug("leader saved proposed block in round state", "roundKey", roundKey)

	if handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		signature, err := handler.signer.Sign(block.Header.HeaderHash)
		if err != nil {
			handler.logger.Error("leader failed to sign own block vote", "roundKey", roundKey, "error", err)
			return err
		}

		selfVote := &data.BlockVote{
			Epoch:     roundKey.Epoch,
			Round:     roundKey.Round,
			MiniRound: roundKey.MiniRound,

			SignerID: handler.signer.ID(),
			VoteType: data.VoteTypeCommit,

			BlockHash:      block.Header.HeaderHash,
			BlockSignature: signature,

			Subdomains:          subdomains,
			SubdomainsSignature: subdomainsSignature,
		}

		err = handler.roundState.AddVote(roundKey, selfVote)
		if err != nil {
			handler.logger.Error("leader failed to add self vote", "roundKey", roundKey, "error", err)
			return err
		}
		handler.logger.Info("miniround1.HandleProposingBlock added leader self vote", "roundKey", roundKey, "signerID", selfVote.SignerID)
	}

	proposedMessage := data.ProposedBlockMessage{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound,
		SenderID:  handler.myID,
		Block:     block,
	}

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.ProposedBlockConsensusMessage,
		ProposedBlockMessage: &proposedMessage,
	}

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	handler.logger.Info("miniround1.HandleProposingBlock broadcasting proposed block", "roundKey", roundKey, "numReceivers", len(validatorsIDs))

	err = handler.broadcaster.BroadcastProposedBlock(consensusMessage, handler.myID, validatorsIDs)
	if err != nil {
		handler.logger.Error("leader failed to broadcast proposed block", "roundKey", roundKey, "error", err)
		return err
	}

	return nil
}

// HandleProposedBlock handles a proposed block message.
// This method will be called by all validators in a specific mini-round.
func (handler *handler) HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error {
	if message == nil {
		handler.logger.Error("received nil proposed block message", "roundKey", roundKey)
		return ErrNilProposedBlockMessage
	}

	proposedBlock := message.Block
	if proposedBlock == nil {
		handler.logger.Error("received proposed block message with nil block", "roundKey", roundKey, "senderID", message.SenderID)
		return ErrNilBlock
	}

	handler.logger.Info(
		"miniround1.HandleProposedBlock received proposed block",
		"roundKey", roundKey,
		"senderID", message.SenderID,
		"numTransactions", len(proposedBlock.Body.Transactions),
	)

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("error when inspecting consensus group", "roundKey", roundKey, "error", err)
		return err
	}

	if message.SenderID != expectedLeader {
		handler.logger.Error("proposed block was not sent by expected leader", "roundKey", roundKey, "senderID", message.SenderID, "expectedLeader", expectedLeader)
		return ErrMessageNotFromLeader
	}

	// validate block and create the hash
	hash, subdomains, subdomainsHash, err := handler.blockValidator.ValidateBlock(proposedBlock)
	if err != nil {
		// should log at least
		handler.logger.Error("proposed block validation failed; vote will not be sent", "roundKey", roundKey, "senderID", message.SenderID, "error", err)
		return nil
	}
	handler.logger.Info("miniround1.HandleProposedBlock validated proposed block", "roundKey", roundKey, "numSubdomainMaps", len(subdomains), "blockHashLen", len(hash), "subdomainsHashLen", len(subdomainsHash))

	// save the block in the round state (will be needed later)
	err = handler.roundState.SetProposedBlock(roundKey, proposedBlock)
	if err != nil {
		handler.logger.Error("failed to save proposed block in round state", "roundKey", roundKey, "error", err)
		return err
	}

	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		handler.logger.Info("miniround1.HandleProposedBlock node is not in consensus group; no vote sent", "roundKey", roundKey)
		return nil
	}

	// create the blockSignature of the block
	blockSignature, err := handler.signer.Sign(hash)
	if err != nil {
		handler.logger.Error("failed to sign proposed block hash", "roundKey", roundKey, "error", err)
		return err
	}

	// create the blockSignature of the subdomains
	subdomainsSignature, err := handler.signer.Sign(subdomainsHash)
	if err != nil {
		handler.logger.Error("failed to sign subdomains hash", "roundKey", roundKey, "error", err)
		return err
	}

	vote := &data.BlockVote{
		Epoch:     message.Epoch,
		Round:     message.Round,
		MiniRound: message.MiniRound,

		SignerID: handler.signer.ID(),
		VoteType: data.VoteTypeCommit,

		BlockHash:      hash,
		BlockSignature: blockSignature,

		Subdomains:          subdomains,
		SubdomainsSignature: subdomainsSignature,
	}

	leader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("failed to get leader before sending vote", "roundKey", roundKey, "error", err)
		return err
	}

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.BlockVoteConsensusMessage,
		BlockVote:            vote,
	}

	handler.logger.Info("miniround1.HandleProposedBlock sending block vote to leader", "roundKey", roundKey, "leaderID", leader, "signerID", vote.SignerID)
	return handler.broadcaster.SendVoteToLeader(consensusMessage, leader)
}

// HandleBlockVote handles a block vote.
// This method will be called by the leader of a specific mini-round each time it receives a new vote.
func (handler *handler) HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	if vote == nil {
		handler.logger.Error("leader received nil block vote", "roundKey", roundKey)
		return ErrNilVote
	}

	handler.logger.Info("miniround1.HandleBlockVote leader received block vote", "roundKey", roundKey, "signerID", vote.SignerID)

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("leader failed to get selected leader while handling vote", "roundKey", roundKey, "error", err)
		return err
	}

	if leaderID != handler.myID {
		handler.logger.Error("non-leader attempted to collect votes", "roundKey", roundKey, "expectedLeader", leaderID, "localNode", handler.myID)
		return ErrOnlyLeaderCanCollectVotes
	}

	// TODO should also verify the domains. if the domains proposed by the validator are not ok, skip the vote entirely.
	// TODO if vote is not ok, should be skipped. check if returning error here is ok or should do something else.
	err = handler.verifyBlockVote(roundKey, vote)
	if err != nil {
		handler.logger.Error("block vote verification failed", "roundKey", roundKey, "signerID", vote.SignerID, "error", err)
		return err
	}
	handler.logger.Debug("block vote verified", "roundKey", roundKey, "signerID", vote.SignerID)

	err = handler.roundState.AddVote(roundKey, vote)
	if err != nil {
		handler.logger.Error("failed to add block vote to round state", "roundKey", roundKey, "signerID", vote.SignerID, "error", err)
		return err
	}

	votes, err := handler.roundState.GetVotes(roundKey)
	if err != nil {
		handler.logger.Error("failed to get votes from round state", "roundKey", roundKey, "error", err)
		return err
	}

	consensusGroupSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		handler.logger.Error("failed to get consensus group size", "roundKey", roundKey, "error", err)
		return err
	}

	// Wait for all G consensus-group members to vote so that Byzantine validators
	// that respond instantly cannot crowd out honest validators in a smaller pool.
	// HandleVoteCollectionTimeout provides the fallback when the deadline fires.
	if uint64(len(votes)) < consensusGroupSize || handler.roundState.IsCertificateSet(roundKey) {
		handler.logger.Info(
			"leader waiting for more votes or certificate already set",
			"roundKey", roundKey,
			"numVotes", len(votes),
			"consensusGroupSize", consensusGroupSize,
			"certificateSet", handler.roundState.IsCertificateSet(roundKey),
		)
		return nil
	}
	handler.logger.Info("miniround1.HandleBlockVote leader collected all consensus-group votes", "roundKey", roundKey, "numVotes", len(votes))

	return handler.aggregateAndFinalize(roundKey, votes, consensusGroupSize)
}

// HandleVoteCollectionTimeout is called when the vote-collection deadline expires
// before all G consensus-group members have voted. If at least Q votes are present
// the leader aggregates with what it has; otherwise the round cannot be finalized.
func (handler *handler) HandleVoteCollectionTimeout(roundKey data.RoundKey) error {
	if handler.roundState.IsCertificateSet(roundKey) {
		handler.logger.Info("vote-collection timeout: certificate already set, nothing to do", "roundKey", roundKey)
		return nil
	}

	votes, err := handler.roundState.GetVotes(roundKey)
	if err != nil {
		handler.logger.Error("vote-collection timeout: failed to get votes", "roundKey", roundKey, "error", err)
		return err
	}

	consensusGroupSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		handler.logger.Error("vote-collection timeout: failed to get consensus group size", "roundKey", roundKey, "error", err)
		return err
	}

	quorum := (2*consensusGroupSize)/3 + 1
	if uint64(len(votes)) < quorum {
		handler.logger.Info("vote-collection timeout: insufficient votes to finalize",
			"roundKey", roundKey, "numVotes", len(votes), "quorum", quorum)
		return ErrNotEnoughVotesAfterTimeout
	}

	handler.logger.Info("vote-collection timeout: aggregating with available votes",
		"roundKey", roundKey, "numVotes", len(votes), "quorum", quorum, "consensusGroupSize", consensusGroupSize)

	return handler.aggregateAndFinalize(roundKey, votes, consensusGroupSize)
}

func (handler *handler) aggregateAndFinalize(roundKey data.RoundKey, votes []*data.ValidatorVote, consensusGroupSize uint64) error {
	currentProposedBlock, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		handler.logger.Error("leader failed to get proposed block for aggregation", "roundKey", roundKey, "error", err)
		return err
	}
	hash := currentProposedBlock.Header.HeaderHash

	signers, signatures, subdomains, subdomainsSignatures, err := handler.extractSignersAndVotes(votes)
	if err != nil {
		handler.logger.Error("leader failed to extract signers and votes", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Debug("leader extracted aggregated votes", "roundKey", roundKey, "numSigners", len(signers), "numSubdomainMaps", len(subdomains))

	aggVotes := data.AggregatedVotes{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound,

		SenderID: handler.myID,

		BlockHash:  hash,
		Signers:    signers,
		Signatures: signatures,

		Subdomains:           subdomains,
		SubdomainsSignatures: subdomainsSignatures,
	}

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.AggregatedVotesConsensusMessage,
		AggregatedVotes:      &aggVotes,
	}

	err = handler.roundState.SetCertificate(roundKey, &aggVotes)
	if err != nil {
		handler.logger.Error("leader failed to set round certificate", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Info("leader set round certificate", "roundKey", roundKey, "numSigners", len(aggVotes.Signers))

	subdomainsFrequencies, nonRelatedTxHashes, dominantLabelsPerTx, err := handler.labelsValidator.AggregateLabels(aggVotes.Subdomains, consensusGroupSize)
	if err != nil {
		handler.logger.Error("leader failed to aggregate labels", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Info("leader aggregated subdomain frequencies", "roundKey", roundKey, "frequencies", subdomainsFrequencies, "numNonRelatedTxs", len(nonRelatedTxHashes))

	for _, tx := range currentProposedBlock.Body.Transactions {
		if labels, ok := dominantLabelsPerTx[string(tx.GetTxHash())]; ok {
			tx.SetDomainLabels(labels)
		}
	}

	err = handler.blockFinalizer.FinalizeBlockMROne(roundKey, &data.BlockOnChain{
		Block:                       *currentProposedBlock,
		SubdomainsFrequencies:       subdomainsFrequencies,
		NonRelatedTransactionHashes: nonRelatedTxHashes,
	})
	if err != nil {
		handler.logger.Error("leader failed to finalize block", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Info("leader finalized block", "roundKey", roundKey, "numFrequencies", len(subdomainsFrequencies))

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	handler.logger.Info("leader broadcasting aggregated votes", "roundKey", roundKey, "numReceivers", len(validatorsIDs))

	return handler.broadcaster.BroadcastAggregatedVotes(consensusMessage, handler.myID, validatorsIDs)
}

func (handler *handler) verifyBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	signature := vote.BlockSignature
	validatorID := vote.SignerID

	isValidator := handler.validatorRegistry.IsValidatorRegistered(validatorID)
	if !isValidator {
		handler.logger.Error("vote signer is not registered", "roundKey", roundKey, "signerID", validatorID)
		return ErrSignerIsNotValidator
	}

	isValidatorInConsensusGroup := handler.validatorRegistry.IsValidatorInConsensusGroup(validatorID)
	if !isValidatorInConsensusGroup {
		handler.logger.Error("vote signer is not in consensus group", "roundKey", roundKey, "signerID", validatorID)
		return ErrValidatorNotPartOfConsensusGroup
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(validatorID)
	if err != nil {
		handler.logger.Error("failed to get signer public key", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	currentProposedBlock, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		handler.logger.Error("failed to get proposed block for vote verification", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	hash := currentProposedBlock.Header.HeaderHash
	err = handler.signer.Verify(publicKey, hash, signature)
	if err != nil {
		handler.logger.Error("block signature verification failed", "roundKey", roundKey, "signerID", validatorID, "error", err)
		return err
	}

	return nil
}

func (handler *handler) extractSignersAndVotes(votes []*data.ValidatorVote) ([][]byte, [][]byte, []data.Subdomains, [][]byte, error) {
	publicKeys := make([][]byte, 0, len(votes))
	blockSignatures := make([][]byte, 0, len(votes))
	subdomains := make([]data.Subdomains, 0, len(votes))
	subdomainsSignatures := make([][]byte, 0, len(votes))

	for _, vote := range votes {
		validatorID := vote.ValidatorID

		publicKey, err := handler.validatorRegistry.GetPublicKey(validatorID)
		if err != nil {
			handler.logger.Error("failed to get public key while extracting votes", "validatorID", validatorID, "error", err)
			return nil, nil, nil, nil, err
		}

		publicKeys = append(publicKeys, publicKey)
		blockSignatures = append(blockSignatures, vote.BlockSignature)
		subdomains = append(subdomains, vote.Subdomains)
		subdomainsSignatures = append(subdomainsSignatures, vote.SubdomainsSignature)
	}

	return publicKeys, blockSignatures, subdomains, subdomainsSignatures, nil
}

// HandleAggregatedVotes handles the aggregated votes created by the leader.
// This method will be called by each validator of the consensus group of a specific mini-round.
// TODO This method should be more aggressive!
func (handler *handler) HandleAggregatedVotes(roundKey data.RoundKey, votes *data.AggregatedVotes) error {
	handler.logger.Info("miniround1.HandleAggregatedVotes received aggregated votes", "roundKey", roundKey)

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("failed to get leader while handling aggregated votes", "roundKey", roundKey, "error", err)
		return err
	}

	if votes.SenderID != expectedLeader {
		handler.logger.Error("aggregated votes not from expected leader", "roundKey", roundKey, "senderID", votes.SenderID, "expectedLeader", expectedLeader)
		return ErrMessageNotFromLeader
	}
	handler.logger.Info("miniround1.HandleAggregatedVotes verified sender", "roundKey", roundKey, "senderID", votes.SenderID, "numSigners", len(votes.Signers))

	block, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		handler.logger.Error("failed to get proposed block while handling aggregated votes", "roundKey", roundKey, "error", err)
		return err
	}
	hash := block.Header.HeaderHash

	signers := votes.Signers
	blockSignatures := votes.Signatures
	aggregatedSubdomains := votes.Subdomains
	aggregatedSubdomainsSignatures := votes.SubdomainsSignatures

	for i, blockSignature := range blockSignatures {
		handler.logger.Debug("verifying aggregated block signature", "roundKey", roundKey, "index", i)
		err = handler.verifySignature(hash, blockSignature, signers[i])
		if err != nil {
			handler.logger.Error("aggregated block signature verification failed", "roundKey", roundKey, "index", i, "error", err)
			return fmt.Errorf("block signature error %s", err.Error())
		}
	}

	for i, subdomainsSignature := range aggregatedSubdomainsSignatures {
		handler.logger.Debug("validating aggregated subdomains", "roundKey", roundKey, "index", i, "numTransactions", len(aggregatedSubdomains[i]))
		err = handler.labelsValidator.ValidateLabels(aggregatedSubdomains[i])
		if err != nil {
			handler.logger.Error("aggregated subdomain labels validation failed", "roundKey", roundKey, "index", i, "error", err)
			return err
		}

		subdomainsHash, err := hashing.ComputeSubdomainsHash(aggregatedSubdomains[i])
		if err != nil {
			handler.logger.Error("failed to hash aggregated subdomains", "roundKey", roundKey, "index", i, "error", err)
			return err
		}

		err = handler.verifySignature(subdomainsHash, subdomainsSignature, signers[i])
		if err != nil {
			handler.logger.Error("aggregated subdomain signature verification failed", "roundKey", roundKey, "index", i, "error", err)
			return fmt.Errorf("subdomains signature error %s", err.Error())
		}
	}

	consensusGroupSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		handler.logger.Error("failed to get consensus group size while handling aggregated votes", "roundKey", roundKey, "error", err)
		return err
	}

	subdomainsFrequencies, nonRelatedTxHashes, dominantLabelsPerTx, err := handler.labelsValidator.AggregateLabels(aggregatedSubdomains, consensusGroupSize)
	if err != nil {
		handler.logger.Error("failed to aggregate labels from aggregated votes", "roundKey", roundKey, "error", err)
		return err
	}
	handler.logger.Info("miniround1.HandleAggregatedVotes aggregated subdomain frequencies from certificate", "roundKey", roundKey, "frequencies", subdomainsFrequencies, "numNonRelatedTxs", len(nonRelatedTxHashes))

	for _, tx := range block.Body.Transactions {
		if labels, ok := dominantLabelsPerTx[string(tx.GetTxHash())]; ok {
			tx.SetDomainLabels(labels)
		}
	}

	err = handler.blockFinalizer.FinalizeBlockMROne(roundKey, &data.BlockOnChain{
		Block:                       *block,
		SubdomainsFrequencies:       subdomainsFrequencies,
		NonRelatedTransactionHashes: nonRelatedTxHashes,
	})
	if err != nil {
		handler.logger.Error("failed to finalize block from aggregated votes", "roundKey", roundKey, "error", err)
		return err
	}

	handler.logger.Info("miniround1.HandleAggregatedVotes finalized block", "roundKey", roundKey, "numFrequencies", len(subdomainsFrequencies))
	return nil
}

func (handler *handler) verifySignature(hash []byte, signature []byte, signer []byte) error {
	err := handler.signer.Verify(signer, hash, signature)
	if err != nil {
		return err
	}

	return nil
}
