package testscommon

import "moa-chain/data"

type MiniRoundTwoHandlerStub struct {
	HandleConsensusSelectionCalled bool
	HandleConsensusSelectionKey    data.RoundKey
	HandleConsensusSelectionLeader string
	HandleConsensusSelectionErr    error

	HandleBlockExecutionCalled bool
	HandleBlockExecutionKey    data.RoundKey
	HandleBlockExecutionErr    error

	HandleExecutedPromptsMessageCalled  bool
	HandleExecutedPromptsMessageKey     data.RoundKey
	HandleExecutedPromptsMessageMessage *data.AnswersBlockMessage
	HandleExecutedPromptsMessageErr     error

	HandleAnswerEvidenceCalled  bool
	HandleAnswerEvidenceKey     data.RoundKey
	HandleAnswerEvidenceMessage *data.AggregatedExecutionResultsMessage
	HandleAnswerEvidenceErr     error

	HandleAnswerClassificationVoteCalled bool
	HandleAnswerClassificationVoteKey    data.RoundKey
	HandleAnswerClassificationVoteValue  *data.AnswerClassificationVote
	HandleAnswerClassificationVoteErr    error

	HandleClassificationGracePeriodElapsedCalled bool
	HandleClassificationGracePeriodElapsedKey    data.RoundKey
	HandleClassificationGracePeriodElapsedErr    error

	HandleAnswerClassificationCertificateCalled bool
	HandleAnswerClassificationCertificateKey    data.RoundKey
	HandleAnswerClassificationCertificateValue  *data.AnswerClassificationCertificate
	HandleAnswerClassificationCertificateErr    error
	HasVerifiedAnswerEvidenceValue              bool
}

func (stub *MiniRoundTwoHandlerStub) HandleClassificationGracePeriodElapsed(roundKey data.RoundKey) error {
	stub.HandleClassificationGracePeriodElapsedCalled = true
	stub.HandleClassificationGracePeriodElapsedKey = roundKey
	return stub.HandleClassificationGracePeriodElapsedErr
}

func (stub *MiniRoundTwoHandlerStub) HasVerifiedAnswerEvidence(_ data.RoundKey) bool {
	return stub.HasVerifiedAnswerEvidenceValue
}

func (stub *MiniRoundTwoHandlerStub) HandleAnswerEvidence(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	stub.HandleAnswerEvidenceCalled = true
	stub.HandleAnswerEvidenceKey = roundKey
	stub.HandleAnswerEvidenceMessage = message

	return stub.HandleAnswerEvidenceErr
}

func (stub *MiniRoundTwoHandlerStub) HandleAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) error {
	stub.HandleAnswerClassificationVoteCalled = true
	stub.HandleAnswerClassificationVoteKey = roundKey
	stub.HandleAnswerClassificationVoteValue = vote

	return stub.HandleAnswerClassificationVoteErr
}

func (stub *MiniRoundTwoHandlerStub) HandleAnswerClassificationCertificate(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) error {
	stub.HandleAnswerClassificationCertificateCalled = true
	stub.HandleAnswerClassificationCertificateKey = roundKey
	stub.HandleAnswerClassificationCertificateValue = certificate

	return stub.HandleAnswerClassificationCertificateErr
}

func (stub *MiniRoundTwoHandlerStub) HandleConsensusSelection(key data.RoundKey) (string, error) {
	stub.HandleConsensusSelectionCalled = true
	stub.HandleConsensusSelectionKey = key

	return stub.HandleConsensusSelectionLeader, stub.HandleConsensusSelectionErr
}

func (stub *MiniRoundTwoHandlerStub) HandleBlockExecution(roundKey data.RoundKey) error {
	stub.HandleBlockExecutionCalled = true
	stub.HandleBlockExecutionKey = roundKey

	return stub.HandleBlockExecutionErr
}

func (stub *MiniRoundTwoHandlerStub) HandleExecutedPromptsMessage(roundKey data.RoundKey, message *data.AnswersBlockMessage) error {
	stub.HandleExecutedPromptsMessageCalled = true
	stub.HandleExecutedPromptsMessageKey = roundKey
	stub.HandleExecutedPromptsMessageMessage = message

	return stub.HandleExecutedPromptsMessageErr
}

func (stub *MiniRoundTwoHandlerStub) HandleExecutedPromptsCollectionTimeout(_ data.RoundKey) error {
	return nil
}

func (stub *MiniRoundTwoHandlerStub) WaitForPendingWork() {}
