package state

import (
	"errors"
	"sort"

	"moa-chain/data"
)

// roundState defines the round state component.
// It is intended to be owned by a single RoundLoop, which processes inbox
// events sequentially. Because handlers access it through that event loop,
// the internal maps do not need a mutex for the current execution model.
// If round handlers start mutating round state from additional goroutines,
// this component must be protected by synchronization or replaced by an
// event-loop-owned command API.
type roundState struct {
	proposedBlocks             map[data.RoundKey]*data.Block
	votes                      map[data.RoundKey]map[string]*data.BlockVote
	certificates               map[data.RoundKey]*data.AggregatedVotes
	executedPrompts            map[data.RoundKey]map[string]*data.AnswersBlockMessage
	answerEvidence             map[data.RoundKey]*data.AggregatedExecutionResultsMessage
	classificationVotes        map[data.RoundKey]map[string]*data.AnswerClassificationVote
	classificationCertificates map[data.RoundKey]*data.AnswerClassificationCertificate
}

// NewRoundState creates a new round state which caches blocks and votes.
func NewRoundState() *roundState {
	return &roundState{
		proposedBlocks:             make(map[data.RoundKey]*data.Block),
		votes:                      make(map[data.RoundKey]map[string]*data.BlockVote),
		certificates:               make(map[data.RoundKey]*data.AggregatedVotes),
		executedPrompts:            make(map[data.RoundKey]map[string]*data.AnswersBlockMessage),
		answerEvidence:             make(map[data.RoundKey]*data.AggregatedExecutionResultsMessage),
		classificationVotes:        make(map[data.RoundKey]map[string]*data.AnswerClassificationVote),
		classificationCertificates: make(map[data.RoundKey]*data.AnswerClassificationCertificate),
	}
}

// SetAnswerEvidence stores the leader evidence after signature verification and
// before the local validator invokes its answer judge.
func (state *roundState) SetAnswerEvidence(roundKey data.RoundKey, evidence *data.AggregatedExecutionResultsMessage) error {
	if evidence == nil {
		return ErrNilAnswerEvidence
	}
	if _, exists := state.answerEvidence[roundKey]; exists {
		return ErrAnswerEvidenceAlreadyExists
	}

	state.answerEvidence[roundKey] = evidence
	return nil
}

// GetAnswerEvidence returns the verified evidence used by certificate
// verification and mini-round-two finalization.
func (state *roundState) GetAnswerEvidence(roundKey data.RoundKey) (*data.AggregatedExecutionResultsMessage, error) {
	evidence, exists := state.answerEvidence[roundKey]
	if !exists {
		return nil, ErrNoAnswerEvidenceForCurrentRoundKey
	}

	return evidence, nil
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
			ValidatorID:         validatorID,
			BlockSignature:      vote.BlockSignature,
			Subdomains:          vote.Subdomains,
			SubdomainsSignature: vote.SubdomainsSignature,
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

// AddExecutedPromptsMessage stores the executed prompts message received from a validator in mini-round two.
func (state *roundState) AddExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error {
	if message == nil {
		return ErrNilExecutedPromptsMessage
	}

	messagesMap, ok := state.executedPrompts[roundKey]
	if !ok {
		messagesMap = make(map[string]*data.AnswersBlockMessage)
		state.executedPrompts[roundKey] = messagesMap
	}

	_, ok = messagesMap[message.SenderID]
	if ok {
		return ErrExecutedPromptsAlreadyExistsForSender
	}

	messagesMap[message.SenderID] = message
	return nil
}

// GetExecutedPromptsMessages returns the executed prompts messages received in a mini-round two.
func (state *roundState) GetExecutedPromptsMessages(roundKey data.RoundKey) ([]*data.AnswersBlockMessage, error) {
	messages, ok := state.executedPrompts[roundKey]
	if !ok {
		return nil, ErrNoExecutedPromptsForCurrentRoundKey
	}

	senderIDs := make([]string, 0, len(messages))
	for senderID := range messages {
		senderIDs = append(senderIDs, senderID)
	}
	sort.Strings(senderIDs)

	extractedMessages := make([]*data.AnswersBlockMessage, 0, len(senderIDs))
	for _, senderID := range senderIDs {
		extractedMessages = append(extractedMessages, messages[senderID])
	}

	return extractedMessages, nil
}

// AddAnswerClassificationVote stores one classification vote per judge and round.
func (state *roundState) AddAnswerClassificationVote(roundKey data.RoundKey, vote *data.AnswerClassificationVote) error {
	if vote == nil {
		return ErrNilAnswerClassificationVote
	}

	votes, ok := state.classificationVotes[roundKey]
	if !ok {
		votes = make(map[string]*data.AnswerClassificationVote)
		state.classificationVotes[roundKey] = votes
	}
	if _, exists := votes[vote.JudgeID]; exists {
		return ErrAnswerClassificationVoteAlreadyExistsForJudge
	}

	votes[vote.JudgeID] = vote
	return nil
}

// GetAnswerClassificationVotes returns votes in canonical judge-ID order.
func (state *roundState) GetAnswerClassificationVotes(roundKey data.RoundKey) ([]*data.AnswerClassificationVote, error) {
	votes, ok := state.classificationVotes[roundKey]
	if !ok {
		return nil, ErrNoAnswerClassificationVotesForCurrentRoundKey
	}

	judgeIDs := make([]string, 0, len(votes))
	for judgeID := range votes {
		judgeIDs = append(judgeIDs, judgeID)
	}
	sort.Strings(judgeIDs)

	orderedVotes := make([]*data.AnswerClassificationVote, 0, len(judgeIDs))
	for _, judgeID := range judgeIDs {
		orderedVotes = append(orderedVotes, votes[judgeID])
	}

	return orderedVotes, nil
}

// SetAnswerClassificationCertificate stores the leader certificate once per round.
func (state *roundState) SetAnswerClassificationCertificate(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) error {
	if certificate == nil {
		return ErrNilAnswerClassificationCertificate
	}
	if _, exists := state.classificationCertificates[roundKey]; exists {
		return ErrAnswerClassificationCertificateAlreadyExists
	}

	state.classificationCertificates[roundKey] = certificate
	return nil
}

// IsAnswerClassificationCertificateSet reports whether classification vote
// collection already produced a certificate for the round.
func (state *roundState) IsAnswerClassificationCertificateSet(roundKey data.RoundKey) bool {
	_, exists := state.classificationCertificates[roundKey]

	return exists
}

// ClearRoundState clears the state of a round.
func (state *roundState) ClearRoundState(roundKey data.RoundKey) {
	delete(state.proposedBlocks, roundKey)
	delete(state.votes, roundKey)
	delete(state.certificates, roundKey)
	delete(state.executedPrompts, roundKey)
	delete(state.answerEvidence, roundKey)
	delete(state.classificationVotes, roundKey)
	delete(state.classificationCertificates, roundKey)
}
