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

	BroadcastAnswerEvidenceMessage *data.ConsensusMessage
	BroadcastAnswerEvidenceMyID    string
	BroadcastAnswerEvidenceTargets []string
	BroadcastAnswerEvidenceErr     error

	SentClassificationVoteMessage *data.ConsensusMessage
	SentClassificationVoteLeader  string
	SendClassificationVoteErr     error

	BroadcastAnswerClassificationCertificateMessage *data.ConsensusMessage
	BroadcastAnswerClassificationCertificateMyID    string
	BroadcastAnswerClassificationCertificateTargets []string
	BroadcastAnswerClassificationCertificateErr     error

	BroadcastProposedSynthesisMessage *data.ConsensusMessage
	BroadcastProposedSynthesisMyID    string
	BroadcastProposedSynthesisTargets []string
	BroadcastProposedSynthesisErr     error

	SentSynthesisVoteMessage *data.ConsensusMessage
	SentSynthesisVoteLeaderID string
	SendSynthesisVoteErr      error

	BroadcastAggregatedSynthesisVotesMessage *data.ConsensusMessage
	BroadcastAggregatedSynthesisVotesMyID    string
	BroadcastAggregatedSynthesisVotesTargets []string
	BroadcastAggregatedSynthesisVotesErr     error
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

func (stub *BroadcasterStub) BroadcastAnswerEvidence(answerEvidenceMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAnswerEvidenceMessage = answerEvidenceMessage
	stub.BroadcastAnswerEvidenceMyID = myID
	stub.BroadcastAnswerEvidenceTargets = receivers

	return stub.BroadcastAnswerEvidenceErr
}

func (stub *BroadcasterStub) BroadcastAnswerClassificationCertificate(certificateMessage *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAnswerClassificationCertificateMessage = certificateMessage
	stub.BroadcastAnswerClassificationCertificateMyID = myID
	stub.BroadcastAnswerClassificationCertificateTargets = receivers

	return stub.BroadcastAnswerClassificationCertificateErr
}

func (stub *BroadcasterStub) BroadcastProposedSynthesis(msg *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastProposedSynthesisMessage = msg
	stub.BroadcastProposedSynthesisMyID = myID
	stub.BroadcastProposedSynthesisTargets = receivers

	return stub.BroadcastProposedSynthesisErr
}

func (stub *BroadcasterStub) SendSynthesisVoteToLeader(msg *data.ConsensusMessage, leaderID string) error {
	stub.SentSynthesisVoteMessage = msg
	stub.SentSynthesisVoteLeaderID = leaderID

	return stub.SendSynthesisVoteErr
}

func (stub *BroadcasterStub) BroadcastAggregatedSynthesisVotes(msg *data.ConsensusMessage, myID string, receivers []string) error {
	stub.BroadcastAggregatedSynthesisVotesMessage = msg
	stub.BroadcastAggregatedSynthesisVotesMyID = myID
	stub.BroadcastAggregatedSynthesisVotesTargets = receivers

	return stub.BroadcastAggregatedSynthesisVotesErr
}
