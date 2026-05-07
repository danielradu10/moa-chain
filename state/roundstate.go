package state

import (
	"errors"

	"moa-chain/data"
)

// roundState defines the round state component.
type roundState struct {
	proposedBlocks map[data.RoundKey]*data.Block
	votes          map[data.RoundKey]map[string]*data.BlockVote
	certificates   map[data.RoundKey]*data.AggregatedVotes
}

// NewRoundState creates a new round state which caches blocks and votes.
func NewRoundState() *roundState {
	return &roundState{
		proposedBlocks: make(map[data.RoundKey]*data.Block),
		votes:          make(map[data.RoundKey]map[string]*data.BlockVote),
		certificates:   make(map[data.RoundKey]*data.AggregatedVotes),
	}
}

// SetProposedBlock sets the current proposed Block of the RoundKey.
func (state *roundState) SetProposedBlock(roundKey data.RoundKey, block *data.Block) error {
	if block == nil {
		return ErrNilBlock
	}
	_, ok := state.proposedBlocks[roundKey]
	if ok {
		return ErrBlockAlreadyExistsForRoundKey
	}

	state.proposedBlocks[roundKey] = block
	return nil
}

// GetProposedBlock returns the current proposed block of the RoundKey.
func (state *roundState) GetProposedBlock(round data.RoundKey) (*data.Block, error) {
	proposedBlock, ok := state.proposedBlocks[round]
	if !ok {
		return nil, ErrNoProposedBlockForRoundKey
	}

	return proposedBlock, nil
}

// AddVote adds a vote of a specific RoundKey and a specific signer.
func (state *roundState) AddVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	if vote == nil {
		return ErrNilVote
	}

	votesMap, ok := state.votes[roundKey]
	if !ok {
		votesMap = make(map[string]*data.BlockVote)
		state.votes[roundKey] = votesMap
	}

	_, ok = votesMap[vote.SignerID]
	if ok {
		return ErrVoteAlreadyExistsForSigner
	}

	votesMap[vote.SignerID] = vote
	return nil
}

// GetVotes returns the votes of a specific RoundKey.
func (state *roundState) GetVotes(roundKey data.RoundKey) ([]*data.ValidatorVote, error) {
	votes, ok := state.votes[roundKey]
	if !ok {
		return nil, ErrNoVotesForCurrentRoundKey
	}

	extractedVotes := make([]*data.ValidatorVote, 0, len(votes))
	for validatorID, vote := range votes {
		v := &data.ValidatorVote{
			ValidatorID: validatorID,
			Signature:   vote.Signature,
		}

		extractedVotes = append(extractedVotes, v)
	}

	return extractedVotes, nil
}

func (state *roundState) SetCertificate(roundKey data.RoundKey, certificate *data.AggregatedVotes) error {
	_, ok := state.certificates[roundKey]
	if ok {
		return errors.New("duplicate certificate")
	}

	state.certificates[roundKey] = certificate
	return nil
}

func (state *roundState) IsCertificateSet(roundKey data.RoundKey) bool {
	_, ok := state.certificates[roundKey]
	return ok
}

// ClearRoundState clears the state of a round.
func (state *roundState) ClearRoundState(roundKey data.RoundKey) {
	delete(state.proposedBlocks, roundKey)
	delete(state.votes, roundKey)
	delete(state.certificates, roundKey)
}
