package testscommon

import "moa-chain/data"

type MiniRoundOneHandlerStub struct {
	HandleConsensusSelectionCalled bool
	HandleConsensusSelectionKey    data.RoundKey
	HandleConsensusSelectionLeader string
	HandleConsensusSelectionErr    error

	HandleProposingBlockCalled bool
	HandleProposingBlockKey    data.RoundKey
	HandleProposingBlockErr    error

	HandleProposedBlockCalled  bool
	HandleProposedBlockKey     data.RoundKey
	HandleProposedBlockMessage *data.ProposedBlockMessage
	HandleProposedBlockErr     error

	HandleBlockVoteCalled  bool
	HandleBlockVoteKey     data.RoundKey
	HandleBlockVoteMessage *data.BlockVote
	HandleBlockVoteErr     error

	HandleAggregatedVotesCalled  bool
	HandleAggregatedVotesKey     data.RoundKey
	HandleAggregatedVotesMessage *data.AggregatedVotes
	HandleAggregatedVotesErr     error
}

func (stub *MiniRoundOneHandlerStub) HandleConsensusSelection(key data.RoundKey) (string, error) {
	stub.HandleConsensusSelectionCalled = true
	stub.HandleConsensusSelectionKey = key

	return stub.HandleConsensusSelectionLeader, stub.HandleConsensusSelectionErr
}

func (stub *MiniRoundOneHandlerStub) HandleProposingBlock(roundKey data.RoundKey) error {
	stub.HandleProposingBlockCalled = true
	stub.HandleProposingBlockKey = roundKey

	return stub.HandleProposingBlockErr
}

func (stub *MiniRoundOneHandlerStub) HandleProposedBlock(roundKey data.RoundKey, message *data.ProposedBlockMessage) error {
	stub.HandleProposedBlockCalled = true
	stub.HandleProposedBlockKey = roundKey
	stub.HandleProposedBlockMessage = message

	return stub.HandleProposedBlockErr
}

func (stub *MiniRoundOneHandlerStub) HandleBlockVote(roundKey data.RoundKey, vote *data.BlockVote) error {
	stub.HandleBlockVoteCalled = true
	stub.HandleBlockVoteKey = roundKey
	stub.HandleBlockVoteMessage = vote

	return stub.HandleBlockVoteErr
}

func (stub *MiniRoundOneHandlerStub) HandleAggregatedVotes(roundKey data.RoundKey, votes *data.AggregatedVotes) error {
	stub.HandleAggregatedVotesCalled = true
	stub.HandleAggregatedVotesKey = roundKey
	stub.HandleAggregatedVotesMessage = votes

	return stub.HandleAggregatedVotesErr
}
