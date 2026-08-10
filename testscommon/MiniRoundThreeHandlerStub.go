package testscommon

import "moa-chain/data"

type MiniRoundThreeHandlerStub struct {
	HandleConsensusSelectionCalled bool
	HandleConsensusSelectionKey    data.RoundKey
	HandleConsensusSelectionLeader string
	HandleConsensusSelectionErr    error

	HandleSynthesisCalled bool
	HandleSynthesisKey    data.RoundKey
	HandleSynthesisErr    error

	HandleProposedSynthesisCalled bool
	HandleProposedSynthesisKey    data.RoundKey
	HandleProposedSynthesisMsg    *data.ProposedSynthesisMessage
	HandleProposedSynthesisErr    error

	HandleSynthesisVoteCalled bool
	HandleSynthesisVoteKey    data.RoundKey
	HandleSynthesisVoteMsg    *data.SynthesisVote
	HandleSynthesisVoteErr    error

	HandleAggregatedSynthesisVotesCalled bool
	HandleAggregatedSynthesisVotesKey    data.RoundKey
	HandleAggregatedSynthesisVotesMsg    *data.AggregatedSynthesisVotes
	HandleAggregatedSynthesisVotesErr    error
}

func (stub *MiniRoundThreeHandlerStub) HandleConsensusSelection(key data.RoundKey) (string, error) {
	stub.HandleConsensusSelectionCalled = true
	stub.HandleConsensusSelectionKey = key
	return stub.HandleConsensusSelectionLeader, stub.HandleConsensusSelectionErr
}

func (stub *MiniRoundThreeHandlerStub) HandleSynthesis(key data.RoundKey) error {
	stub.HandleSynthesisCalled = true
	stub.HandleSynthesisKey = key
	return stub.HandleSynthesisErr
}

func (stub *MiniRoundThreeHandlerStub) HandleProposedSynthesis(key data.RoundKey, msg *data.ProposedSynthesisMessage) error {
	stub.HandleProposedSynthesisCalled = true
	stub.HandleProposedSynthesisKey = key
	stub.HandleProposedSynthesisMsg = msg
	return stub.HandleProposedSynthesisErr
}

func (stub *MiniRoundThreeHandlerStub) HandleSynthesisVote(key data.RoundKey, vote *data.SynthesisVote) error {
	stub.HandleSynthesisVoteCalled = true
	stub.HandleSynthesisVoteKey = key
	stub.HandleSynthesisVoteMsg = vote
	return stub.HandleSynthesisVoteErr
}

func (stub *MiniRoundThreeHandlerStub) HandleAggregatedSynthesisVotes(key data.RoundKey, msg *data.AggregatedSynthesisVotes) error {
	stub.HandleAggregatedSynthesisVotesCalled = true
	stub.HandleAggregatedSynthesisVotesKey = key
	stub.HandleAggregatedSynthesisVotesMsg = msg
	return stub.HandleAggregatedSynthesisVotesErr
}
