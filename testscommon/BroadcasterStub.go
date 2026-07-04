package testscommon

import "moa-chain/data"

type BroadcasterStub struct {
	SentVoteToLeaderMessage *data.ConsensusMessage
	SentVoteLeaderID        string
	SendVoteToLeaderErr     error

	BroadcastProposedBlockMessage *data.ConsensusMessage
	BroadcastProposedBlockMyID    string
	BroadcastProposedBlockTargets []string
	BroadcastProposedBlockErr     error

	BroadcastAggregatedVotesMessage *data.ConsensusMessage
	BroadcastAggregatedVotesMyID    string
	BroadcastAggregatedVotesTargets []string
	BroadcastAggregatedVotesErr     error

	BroadcastAggregatedExecutionResultsMessage *data.ConsensusMessage
	BroadcastAggregatedExecutionResultsMyID    string
	BroadcastAggregatedExecutionResultsTargets []string
	BroadcastAggregatedExecutionResultsErr     error

	SentClassificationVoteMessage *data.ConsensusMessage
	SentClassificationVoteLeader  string
	SendClassificationVoteErr     error

	BroadcastAnswerClassificationCertificateMessage *data.ConsensusMessage
	BroadcastAnswerClassificationCertificateMyID    string
	BroadcastAnswerClassificationCertificateTargets []string
	BroadcastAnswerClassificationCertificateErr     error
}

func (stub *BroadcasterStub) SendAnswerClassificationVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error {
	stub.SentClassificationVoteMessage = voteMessage
	stub.SentClassificationVoteLeader = leaderID

	return stub.SendClassificationVoteErr
}

func (stub *BroadcasterStub) SendVoteToLeader(voteMessage *data.ConsensusMessage, leaderID string) error {
	stub.SentVoteToLeaderMessage = voteMessage
	stub.SentVoteLeaderID = leaderID

	return stub.SendVoteToLeaderErr
}

func (stub *BroadcasterStub) BroadcastProposedBlock(blockMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastProposedBlockMessage = blockMessage
	stub.BroadcastProposedBlockMyID = myID
	stub.BroadcastProposedBlockTargets = receivers

	return stub.BroadcastProposedBlockErr
}

func (stub *BroadcasterStub) BroadcastAggregatedVotes(aggregatedVotesMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAggregatedVotesMessage = aggregatedVotesMessage
	stub.BroadcastAggregatedVotesMyID = myID
	stub.BroadcastAggregatedVotesTargets = receivers

	return stub.BroadcastAggregatedVotesErr
}

func (stub *BroadcasterStub) BroadcastAggregatedExecutionResults(aggregatedExecutionResultsMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAggregatedExecutionResultsMessage = aggregatedExecutionResultsMessage
	stub.BroadcastAggregatedExecutionResultsMyID = myID
	stub.BroadcastAggregatedExecutionResultsTargets = receivers

	return stub.BroadcastAggregatedExecutionResultsErr
}

func (stub *BroadcasterStub) BroadcastAnswerClassificationCertificate(certificateMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAnswerClassificationCertificateMessage = certificateMessage
	stub.BroadcastAnswerClassificationCertificateMyID = myID
	stub.BroadcastAnswerClassificationCertificateTargets = receivers

	return stub.BroadcastAnswerClassificationCertificateErr
}
