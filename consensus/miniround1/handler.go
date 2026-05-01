package miniround1

import (
	"moa-chain/blockprocessing"
	"moa-chain/broadcast"
	"moa-chain/consensus"
	"moa-chain/crypto/signing"
	"moa-chain/data"
)

type handler struct {
	blockCreator      blockprocessing.BlockCreator
	blockValidator    blockprocessing.BlockProcessor
	roundState        consensus.RoundState
	broadcaster       broadcast.Broadcaster
	signer            signing.MessageSigner
	validatorRegistry consensus.ValidatorRegistry
}

// NewHandler returns a new mini round one handler
func NewHandler() *handler {
	return &handler{}
}

// HandleProposingBlock handles proposing a block.
// This method will be called when a validator becomes leader in a specific mini-round.
func (handler *handler) HandleProposingBlock(roundKey data.RoundKey) error {
	block, err := handler.blockCreator.ProposeBlock()
	if err != nil {
		return err
	}

	// there is no need to process the block, we already process it inside ProposeBlock

	err = handler.roundState.SetProposedBlock(roundKey, block)
	if err != nil {
		return err
	}

	proposedMessage := data.ProposedBlockMessage{
		Epoch:     roundKey.Epoch,
		Round:     roundKey.Round,
		MiniRound: roundKey.MiniRound,
		Block:     block,
	}

	err = handler.broadcaster.BroadcastProposedBlock(&proposedMessage)
	if err != nil {
		return err
	}

	return nil
}

// HandleProposedBlock handles a proposed block message.
// This method will be called by all validators in a specific mini-round.
func (handler *handler) HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error {
	if message == nil {
		return consensus.ErrNilProposedBlockMessage
	}

	proposedBlock := message.Block
	if proposedBlock == nil {
		return consensus.ErrNilBlock
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

	return handler.broadcaster.SendVoteToLeader(vote)
}

// HandleBlockVote handles a block vote.
// This method will be called by the leader of a specific mini-round each time it receives a new vote.
func (handler *handler) HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	if vote == nil {
		return consensus.ErrNilVote
	}

	err := handler.verifyVote(roundKey, vote)
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

	if uint64(len(votes)) < handler.validatorRegistry.ConsensusGroupSize() {
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

	return handler.broadcaster.BroadcastAggregatedVotes(aggVotes)
}

func (handler *handler) verifyVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	signature := vote.Signature
	validatorID := vote.SignerID

	isValidator := handler.validatorRegistry.IsValidatorRegistered(validatorID)
	if !isValidator {
		return consensus.ErrSignerIsNotValidator
	}

	isValidatorInConsensusGroup := handler.validatorRegistry.IsValidatorInConsensusGroup(validatorID)
	if !isValidatorInConsensusGroup {
		return consensus.ErrValidatorNotPartOfConsensusGroup
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
