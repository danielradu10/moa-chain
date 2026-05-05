package miniround1

import (
	"moa-chain/blockprocessing"
	"moa-chain/broadcast"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validators"
)

type handler struct {
	myID string

	blockCreator      blockprocessing.BlockCreator
	blockValidator    blockprocessing.BlockProcessor
	roundState        state.RoundState
	broadcaster       broadcast.Broadcaster
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
	blockchainState   state.BlockchainState
}

// NewHandler returns a new mini round one handler
func NewHandler() *handler {
	return &handler{}
}

// HandleConsensusSelection should be called by each validator in the beginning of the round.
func (handler *handler) HandleConsensusSelection(key data.RoundKey) (string, error) {
	err := handler.validatorRegistry.GenerateConsensusGroup(handler.blockchainState, key)
	if err != nil {
		return "", err
	}

	return handler.validatorRegistry.LeaderOfConsensusGroup()
}

// HandleProposingBlock handles proposing a block.
// This method will be called when a validator becomes leader in a specific mini-round.
func (handler *handler) HandleProposingBlock(roundKey data.RoundKey) error {
	block, err := handler.blockCreator.ProposeBlock()
	if err != nil {
		return err
	}

	// there is no need to process the block, we already processed it inside ProposeBlock

	err = handler.roundState.SetProposedBlock(roundKey, block)
	if err != nil {
		return err
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
	err = handler.broadcaster.BroadcastProposedBlock(consensusMessage, handler.myID, validatorsIDs)
	if err != nil {
		return err
	}

	return nil
}

// HandleProposedBlock handles a proposed block message.
// This method will be called by all validators in a specific mini-round.
func (handler *handler) HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error {
	if message == nil {
		return ErrNilProposedBlockMessage
	}

	proposedBlock := message.Block
	if proposedBlock == nil {
		return ErrNilBlock
	}

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if message.SenderID != expectedLeader {
		return ErrMessageNotFromLeader
	}

	// validate block and create the hash
	hash, err := handler.blockValidator.ValidateBlock(proposedBlock)
	if err != nil {
		return err
	}

	// save the block in the round state (will be needed later)
	err = handler.roundState.SetProposedBlock(roundKey, proposedBlock)
	if err != nil {
		return err
	}

	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		return nil
	}

	// create the signature
	signature, err := handler.signer.Sign(hash)
	if err != nil {
		return err
	}

	vote := &data.BlockVote{
		Epoch:     message.Epoch,
		Round:     message.Round,
		MiniRound: message.MiniRound,

		SignerID: handler.signer.ID(),
		VoteType: data.VoteTypeCommit,

		BlockHash: hash,
		Signature: signature,
	}

	leader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.BlockVoteConsensusMessage,
		BlockVote:            vote,
	}

	return handler.broadcaster.SendVoteToLeader(consensusMessage, string(leader))
}

// HandleBlockVote handles a block vote.
// This method will be called by the leader of a specific mini-round each time it receives a new vote.
func (handler *handler) HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	if vote == nil {
		return ErrNilVote
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}

	if leaderID != handler.myID {
		return ErrOnlyLeaderCanCollectVotes
	}

	err = handler.verifyVote(roundKey, vote)
	if err != nil {
		return err
	}

	err = handler.roundState.AddVote(roundKey, vote)
	if err != nil {
		return err
	}

	votes, err := handler.roundState.GetVotes(roundKey)
	if err != nil {
		return err
	}

	consensusGroupSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}

	if uint64(len(votes)) < (2*consensusGroupSize)/3+1 {
		return nil
	}

	currentProposedBlock, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		return err
	}
	hash := currentProposedBlock.Header.HeaderHash

	signers, signatures, err := handler.extractSignersAndVotes(votes)

	aggVotes := data.AggregatedVotes{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound,

		BlockHash:  hash,
		Signers:    signers,
		Signatures: signatures,
	}

	consensusMessage := &data.ConsensusMessage{
		ConsensusMessageType: data.AggregatedVotesConsensusMessage,
		AggregatedVotes:      &aggVotes,
	}

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	return handler.broadcaster.BroadcastAggregatedVotes(consensusMessage, handler.myID, validatorsIDs)
}

func (handler *handler) verifyVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	signature := vote.Signature
	validatorID := vote.SignerID

	isValidator := handler.validatorRegistry.IsValidatorRegistered(validatorID)
	if !isValidator {
		return ErrSignerIsNotValidator
	}

	isValidatorInConsensusGroup := handler.validatorRegistry.IsValidatorInConsensusGroup(validatorID)
	if !isValidatorInConsensusGroup {
		return ErrValidatorNotPartOfConsensusGroup
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(validatorID)
	if err != nil {
		return err
	}

	currentProposedBlock, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		return err
	}

	hash := currentProposedBlock.Header.HeaderHash
	err = handler.signer.Verify(publicKey, hash, signature)
	if err != nil {
		return err
	}

	return nil
}

func (handler *handler) extractSignersAndVotes(votes []*data.ValidatorVote) ([][]byte, [][]byte, error) {
	publicKeys := make([][]byte, 0, len(votes))
	signatures := make([][]byte, 0, len(votes))

	for _, vote := range votes {
		validatorID := vote.ValidatorID

		publicKey, err := handler.validatorRegistry.GetPublicKey(validatorID)
		if err != nil {
			return nil, nil, err
		}

		publicKeys = append(publicKeys, publicKey)
		signatures = append(signatures, vote.Signature)
	}

	return publicKeys, signatures, nil
}

// HandleAggregatedVotes handles the aggregated votes created by the leader.
// This method will be called by each validator of the consensus group of a specific mini-round.
func (handler *handler) HandleAggregatedVotes(roundKey data.RoundKey, votes *data.AggregatedVotes) error {
	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}

	if votes.SenderID != expectedLeader {
		return ErrMessageNotFromLeader
	}

	block, err := handler.roundState.GetProposedBlock(roundKey)
	if err != nil {
		return err
	}
	hash := block.Header.HeaderHash

	signers := votes.Signers
	signatures := votes.Signatures

	for i, signature := range signatures {
		err = handler.verifySignature(hash, signature, signers[i])
		if err != nil {
			return err
		}
	}

	return nil
}

func (handler *handler) verifySignature(hash []byte, signature []byte, signer []byte) error {
	err := handler.signer.Verify(signer, hash, signature)
	if err != nil {
		return err
	}

	return nil
}
