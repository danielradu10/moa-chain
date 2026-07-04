package miniround2

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
)

func TestHandleAnswerEvidenceForClassificationSendsSignedVoteToLeader(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)

	err := context.handler.HandleAnswerEvidenceForClassification(context.roundKey, context.evidence)

	require.NoError(t, err)
	require.Len(t, judge.inputs, 2)
	for _, input := range judge.inputs {
		require.NotContains(t, input.UserPrompt, "validator-a")
		require.NotContains(t, input.UserPrompt, "leader")
	}
	require.Equal(t, "leader", context.broadcaster.SentClassificationVoteLeader)
	require.NotNil(t, context.broadcaster.SentClassificationVoteMessage)

	vote := context.broadcaster.SentClassificationVoteMessage.AnswerClassificationVote
	require.NotNil(t, vote)
	require.Equal(t, "validator-a", vote.JudgeID)
	require.Equal(t, "test-model", vote.ModelMetadata)
	require.Len(t, vote.Assignments, 6)
	require.Equal(t, AnswerJudgePromptVersion, vote.PromptVersion)
	require.Equal(t, AnswerJudgePromptHash(), vote.PromptHash)

	expectedEvidenceHash, err := hashing.ComputeAnswerEvidenceHash(context.evidence)
	require.NoError(t, err)
	require.Equal(t, expectedEvidenceHash, vote.AnswerEvidenceHash)
	expectedVoteHash, err := hashing.ComputeClassificationVoteHash(vote)
	require.NoError(t, err)
	require.Equal(t, expectedVoteHash, vote.VoteHash)
	require.NoError(t, context.signer.Verify(
		context.publicKeys["validator-a"], vote.VoteHash, vote.Signature,
	))
	require.Equal(t, 1, context.signer.signCalls)

	localAnswers := 0
	for _, assignment := range vote.Assignments {
		if assignment.CandidateID.ProducerID == "validator-a" {
			localAnswers++
		}
	}
	require.Equal(t, 2, localAnswers, "the judge must classify its own answer for each transaction")
}

func TestHandleAnswerEvidenceForClassificationStoresLeaderVoteThroughHandler(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})

	err := context.handler.HandleAnswerEvidenceForClassification(context.roundKey, context.evidence)

	require.NoError(t, err)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
	votes, err := context.roundState.GetAnswerClassificationVotes(context.roundKey)
	require.NoError(t, err)
	require.Len(t, votes, 1)
	require.Equal(t, "leader", votes[0].JudgeID)
	require.Equal(t, 1, context.signer.signCalls)
}

func TestHandleAnswerEvidenceForClassificationVerifiesEvidenceBeforeJudgingOrSigning(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)
	context.evidence.BlockSignatures[0] = append([]byte(nil), context.evidence.BlockSignatures[0]...)
	context.evidence.BlockSignatures[0][0] ^= 0xff

	err := context.handler.HandleAnswerEvidenceForClassification(context.roundKey, context.evidence)

	require.Error(t, err)
	require.Empty(t, judge.inputs)
	require.Equal(t, 0, context.signer.signCalls)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
}

func TestHandleAnswerEvidenceForClassificationDiscardsPartialJudgeResult(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{failAtCall: 2}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)

	err := context.handler.HandleAnswerEvidenceForClassification(context.roundKey, context.evidence)

	require.ErrorIs(t, err, ErrAnswerJudgeExecutionFailed)
	require.Len(t, judge.inputs, 2)
	require.Equal(t, 0, context.signer.signCalls)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
}

func TestSignAnswerClassificationVoteRejectsUnexpectedEvidenceAndPrompt(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "validator-a", "leader", &classificationProductionJudge{})
	expectedEvidenceHash := []byte("expected-evidence")
	vote := &data.AnswerClassificationVote{
		AnswerEvidenceHash: expectedEvidenceHash,
		PromptVersion:      AnswerJudgePromptVersion,
		PromptHash:         AnswerJudgePromptHash(),
	}

	vote.AnswerEvidenceHash = []byte("different-evidence")
	err := context.handler.signAnswerClassificationVote(vote, expectedEvidenceHash)
	require.ErrorIs(t, err, ErrClassificationVoteEvidenceMismatch)
	require.Equal(t, 0, context.signer.signCalls)

	vote.AnswerEvidenceHash = expectedEvidenceHash
	vote.PromptHash = []byte("different-prompt")
	err = context.handler.signAnswerClassificationVote(vote, expectedEvidenceHash)
	require.ErrorIs(t, err, ErrClassificationVotePromptMismatch)
	require.Equal(t, 0, context.signer.signCalls)
}

type classificationProductionContext struct {
	roundKey      data.RoundKey
	handler       *miniRoundTwoHandler
	evidence      *data.AggregatedExecutionResultsMessage
	signer        *recordingMessageSigner
	publicKeys    map[string][]byte
	memberSigners map[string]signing.MessageSigner
	registry      *testscommon.ValidatorRegistryStub
	broadcaster   *testscommon.BroadcasterStub
	roundState    state.RoundState
}

func newClassificationProductionContext(
	t *testing.T,
	myID string,
	leaderID string,
	judge agent.AnswerJudge,
) classificationProductionContext {
	t.Helper()

	roundKey := createTestRoundKey()
	finalizedBlock := createTestFinalizedBlock()
	producerIDs := []string{"leader", "validator-a", "validator-b"}
	publicKeys := make(map[string][]byte, len(producerIDs))
	producerSigners := make(map[string]signing.MessageSigner, len(producerIDs))
	messages := make([]*data.AnswersBlockMessage, 0, len(producerIDs))
	for _, producerID := range producerIDs {
		publicKey, privateKey := createTestKeyPair(t)
		producerSigner := signing.NewSigner(producerID, privateKey)
		publicKeys[producerID] = publicKey
		producerSigners[producerID] = producerSigner
		messages = append(messages, createSignedExecutedPromptsMessage(t, producerSigner, finalizedBlock))
	}

	recordingSigner := &recordingMessageSigner{delegate: producerSigners[myID]}
	roundState := state.NewRoundState()
	broadcaster := &testscommon.BroadcasterStub{}
	registry := &testscommon.ValidatorRegistryStub{
		RegisteredValidators:    map[string]bool{"leader": true, "validator-a": true, "validator-b": true},
		ConsensusValidators:     map[string]bool{"leader": true, "validator-a": true, "validator-b": true},
		PublicKeysByValidatorID: publicKeys,
		LeaderID:                leaderID,
		ConsensusGroupSizeValue: 3,
		ConsensusGroupValue:     producerIDs,
		ValidatorsIDs:           producerIDs,
	}
	handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
		myID:               myID,
		signer:             recordingSigner,
		answerJudge:        judge,
		judgeModelMetadata: "test-model",
		roundState:         roundState,
		broadcaster:        broadcaster,
		blockFinalizer:     createSeededFinalizer(t, finalizedBlock),
		validatorRegistry:  registry,
	})

	return classificationProductionContext{
		roundKey:      roundKey,
		handler:       handler,
		evidence:      createAggregatedExecutionResultsMessageFromExecutedPrompts(leaderID, roundKey, messages...),
		signer:        recordingSigner,
		publicKeys:    publicKeys,
		memberSigners: producerSigners,
		registry:      registry,
		broadcaster:   broadcaster,
		roundState:    roundState,
	}
}

type classificationProductionJudge struct {
	inputs     []agent.AnswerJudgeRequest
	failAtCall int
}

func (judge *classificationProductionJudge) JudgeAnswers(input agent.AnswerJudgeRequest) (string, error) {
	judge.inputs = append(judge.inputs, input)
	if judge.failAtCall == len(judge.inputs) {
		return "", errors.New("judge failed")
	}

	return `{"classifications":[` +
		`{"candidateId":"candidate-1","category":"CORRECT"},` +
		`{"candidateId":"candidate-2","category":"CORRECT"},` +
		`{"candidateId":"candidate-3","category":"CORRECT"}]}`, nil
}

type recordingMessageSigner struct {
	delegate  signing.MessageSigner
	signCalls int
}

func (signer *recordingMessageSigner) ID() string {
	return signer.delegate.ID()
}

func (signer *recordingMessageSigner) Sign(message []byte) ([]byte, error) {
	signer.signCalls++
	return signer.delegate.Sign(message)
}

func (signer *recordingMessageSigner) Verify(publicKey []byte, message []byte, signature []byte) error {
	return signer.delegate.Verify(publicKey, message, signature)
}

func (signer *recordingMessageSigner) IsInterfaceNil() bool {
	return signer == nil
}
