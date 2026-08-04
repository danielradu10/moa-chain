package miniround2

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/consensus/miniround2/classification"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
)

func TestHandleAnswerEvidenceSendsSignedVoteToLeader(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)

	err := context.handler.HandleAnswerEvidence(context.roundKey, context.evidence)

	require.NoError(t, err)
	context.handler.judgeWg.Wait()
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
	require.Len(t, vote.AnswerClassifications, 6)
	require.Equal(t, classification.AnswerJudgePromptVersion, vote.PromptVersion)
	require.Equal(t, classification.AnswerJudgePromptHash(), vote.PromptHash)

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
	for _, classification := range vote.AnswerClassifications {
		if classification.CandidateID.ProducerID == "validator-a" {
			localAnswers++
		}
	}
	require.Equal(t, 2, localAnswers, "the judge must classify its own answer for each transaction")

	finalizedBlock, finalizationErr := context.handler.blockFinalizer.GetFinalizedBlockInMRTwo(context.roundKey)
	require.Nil(t, finalizedBlock)
	require.ErrorIs(t, finalizationErr, blockFinalizer.ErrFinalizedBlockNotFound)
}

func TestHandleAnswerEvidenceStoresLeaderVoteThroughHandler(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})

	err := context.handler.HandleAnswerEvidence(context.roundKey, context.evidence)

	require.NoError(t, err)
	// classificationLeaderVote waits for the goroutine and drains selfInbox.
	vote := classificationLeaderVote(t, context)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
	require.Equal(t, "leader", vote.JudgeID)
	require.Equal(t, 1, context.signer.signCalls)
}

func TestHandleAnswerEvidenceVerifiesEvidenceBeforeJudgingOrSigning(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)
	context.evidence.BlockSignatures[0] = append([]byte(nil), context.evidence.BlockSignatures[0]...)
	context.evidence.BlockSignatures[0][0] ^= 0xff

	err := context.handler.HandleAnswerEvidence(context.roundKey, context.evidence)

	require.Error(t, err)
	require.Empty(t, judge.inputs)
	require.Equal(t, 0, context.signer.signCalls)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
}

func TestHandleAnswerEvidenceDiscardsPartialJudgeResultWithoutFailingRound(t *testing.T) {
	t.Parallel()

	judge := &classificationProductionJudge{failAtCall: 2}
	context := newClassificationProductionContext(t, "validator-a", "leader", judge)

	err := context.handler.HandleAnswerEvidence(context.roundKey, context.evidence)

	require.NoError(t, err)
	context.handler.judgeWg.Wait()
	require.Len(t, judge.inputs, 2)
	require.Equal(t, 0, context.signer.signCalls)
	require.Nil(t, context.broadcaster.SentClassificationVoteMessage)
	storedEvidence, err := context.roundState.GetAnswerEvidence(context.roundKey)
	require.NoError(t, err)
	require.Same(t, context.evidence, storedEvidence)
}

func TestSignAnswerClassificationVoteRejectsUnexpectedEvidenceAndPrompt(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "validator-a", "leader", &classificationProductionJudge{})
	expectedEvidenceHash := []byte("expected-evidence")
	vote := &data.AnswerClassificationVote{
		AnswerEvidenceHash: expectedEvidenceHash,
		PromptVersion:      classification.AnswerJudgePromptVersion,
		PromptHash:         classification.AnswerJudgePromptHash(),
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
	selfInbox     chan data.RoundEvent
}

func newClassificationProductionContext(
	t *testing.T,
	myID string,
	leaderID string,
	judge agent.AnswersJudge,
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
	selfInbox := make(chan data.RoundEvent, 32)
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
		selfInbox:          selfInbox,
	})
	t.Cleanup(func() { handler.judgeWg.Wait() })

	return classificationProductionContext{
		roundKey:      roundKey,
		handler:       handler,
		evidence:      createAnswerEvidenceFromExecutedPrompts(leaderID, roundKey, messages...),
		signer:        recordingSigner,
		publicKeys:    publicKeys,
		memberSigners: producerSigners,
		registry:      registry,
		broadcaster:   broadcaster,
		roundState:    roundState,
		selfInbox:     selfInbox,
	}
}

type classificationProductionJudge struct {
	inputs     []agent.AnswerJudgeRequest
	failAtCall int
}

func (judge *classificationProductionJudge) JudgeTransactionAnswers(input agent.AnswerJudgeRequest) (string, error) {
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

func TestHandleAnswerClassificationVoteBroadcastsCertificateAtQuorum(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))

	leaderVote := classificationLeaderVote(t, context)
	validatorBVote := signedClassificationVote(t, leaderVote, "validator-b", context.memberSigners["validator-b"])
	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, validatorBVote))
	require.Nil(t, context.broadcaster.BroadcastAnswerClassificationCertificateMessage)

	validatorAVote := signedClassificationVote(t, leaderVote, "validator-a", context.memberSigners["validator-a"])
	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, validatorAVote))

	message := context.broadcaster.BroadcastAnswerClassificationCertificateMessage
	require.NotNil(t, message)
	require.Equal(t, data.AnswerClassificationCertificateConsensusMessage, message.ConsensusMessageType)
	require.Equal(t, "leader", context.broadcaster.BroadcastAnswerClassificationCertificateMyID)
	require.Equal(t, []string{"leader", "validator-a", "validator-b"}, context.broadcaster.BroadcastAnswerClassificationCertificateTargets)

	certificate := message.AnswerClassificationCertificate
	require.NotNil(t, certificate)
	require.Equal(t, []string{"leader", "validator-a", "validator-b"}, classificationJudgeIDs(certificate.Votes))
	expected, err := classification.AggregateClassificationVotes(
		classificationCandidateIDs(leaderVote.AnswerClassifications), certificate.Votes, 3,
	)
	require.NoError(t, err)
	require.Equal(t, expected, certificate.Transactions)
	require.True(t, context.roundState.IsAnswerClassificationCertificateSet(context.roundKey))
}

func TestClassificationGracePeriodBuildsCertificateOnExpiry(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	context.registry.ConsensusGroupSizeValue = 4
	context.handler.classificationGracePeriod = time.Minute
	context.handler.classificationGraceStarted = make(map[data.RoundKey]time.Time)
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))

	leaderVote := classificationLeaderVote(t, context)
	for _, judgeID := range []string{"validator-a", "validator-b"} {
		vote := signedClassificationVote(t, leaderVote, judgeID, context.memberSigners[judgeID])
		require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	}
	require.Nil(t, context.broadcaster.BroadcastAnswerClassificationCertificateMessage)

	require.NoError(t, context.handler.HandleClassificationGracePeriodElapsed(context.roundKey))
	certificate := context.broadcaster.BroadcastAnswerClassificationCertificateMessage.AnswerClassificationCertificate
	require.Len(t, certificate.Votes, 3)
}

func TestClassificationGracePeriodBuildsImmediatelyWhenAllVotesArrive(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	publicKey, privateKey := createTestKeyPair(t)
	validatorCSigner := signing.NewSigner("validator-c", privateKey)
	context.registry.RegisteredValidators["validator-c"] = true
	context.registry.ConsensusValidators["validator-c"] = true
	context.registry.PublicKeysByValidatorID["validator-c"] = publicKey
	context.registry.ConsensusGroupSizeValue = 4
	context.registry.ConsensusGroupValue = append(context.registry.ConsensusGroupValue, "validator-c")
	context.registry.ValidatorsIDs = append(context.registry.ValidatorsIDs, "validator-c")
	context.handler.classificationGracePeriod = time.Minute
	context.handler.classificationGraceStarted = make(map[data.RoundKey]time.Time)
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))

	leaderVote := classificationLeaderVote(t, context)
	for _, judgeID := range []string{"validator-a", "validator-b"} {
		vote := signedClassificationVote(t, leaderVote, judgeID, context.memberSigners[judgeID])
		require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	}
	require.Nil(t, context.broadcaster.BroadcastAnswerClassificationCertificateMessage)

	validatorCVote := signedClassificationVote(t, leaderVote, "validator-c", validatorCSigner)
	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, validatorCVote))
	certificate := context.broadcaster.BroadcastAnswerClassificationCertificateMessage.AnswerClassificationCertificate
	require.Len(t, certificate.Votes, 4)
}

func TestHandleAnswerClassificationVoteIgnoresInvalidExternalVotes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mutate      func(*data.AnswerClassificationVote)
		resign      bool
		targetError error
	}{
		{
			name:        "judge outside committee",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.JudgeID = "outsider" },
			targetError: ErrValidatorNotPartOfConsensusGroup,
		},
		{
			name:        "invalid signature",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.Signature[0] ^= 0xff },
			targetError: signing.ErrWrongSignature,
		},
		{
			name:        "different evidence",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.AnswerEvidenceHash = []byte("different-evidence") },
			resign:      true,
			targetError: classification.ErrClassificationVoteContextMismatch,
		},
		{
			name:        "different prompt version",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.PromptVersion = "different-version" },
			resign:      true,
			targetError: ErrClassificationVotePromptMismatch,
		},
		{
			name:        "different prompt hash",
			mutate:      func(vote *data.AnswerClassificationVote) { vote.PromptHash = []byte("different-prompt") },
			resign:      true,
			targetError: ErrClassificationVotePromptMismatch,
		},
		{
			name: "missing candidate",
			mutate: func(vote *data.AnswerClassificationVote) {
				vote.AnswerClassifications = vote.AnswerClassifications[:len(vote.AnswerClassifications)-1]
			},
			resign:      true,
			targetError: classification.ErrMissingAnswerCandidate,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
			require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
			vote := signedClassificationVote(t, classificationLeaderVote(t, context), "validator-a", context.memberSigners["validator-a"])
			test.mutate(vote)
			if test.resign {
				signClassificationVote(t, vote, context.memberSigners["validator-a"])
			}

			err := context.handler.HandleAnswerClassificationVote(context.roundKey, vote)

			require.NoError(t, err)
			votes, getErr := context.roundState.GetAnswerClassificationVotes(context.roundKey)
			require.NoError(t, getErr)
			require.Len(t, votes, 1)
			require.Equal(t, context.handler.myID, votes[0].JudgeID)
		})
	}
}

func TestHandleAnswerClassificationVoteRejectsDuplicateJudge(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
	vote := signedClassificationVote(t, classificationLeaderVote(t, context), "validator-a", context.memberSigners["validator-a"])

	require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	require.ErrorIs(
		t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote),
		state.ErrAnswerClassificationVoteAlreadyExistsForJudge,
	)
}

func TestClassificationCertificateIsIndependentOfVoteArrivalOrder(t *testing.T) {
	t.Parallel()

	first := classificationCertificateForArrivalOrder(t, []string{"validator-a", "validator-b"})
	second := classificationCertificateForArrivalOrder(t, []string{"validator-b", "validator-a"})

	require.Equal(t, first.Transactions, second.Transactions)
	require.Equal(t, classificationJudgeIDs(first.Votes), classificationJudgeIDs(second.Votes))
}

func classificationCertificateForArrivalOrder(
	t *testing.T,
	judgeIDs []string,
) *data.AnswerClassificationCertificate {
	t.Helper()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
	leaderVote := classificationLeaderVote(t, context)
	for _, judgeID := range judgeIDs {
		vote := signedClassificationVote(t, leaderVote, judgeID, context.memberSigners[judgeID])
		require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	}

	return context.broadcaster.BroadcastAnswerClassificationCertificateMessage.AnswerClassificationCertificate
}

func classificationLeaderVote(
	t *testing.T,
	context classificationProductionContext,
) *data.AnswerClassificationVote {
	t.Helper()
	// Wait for the async judge goroutine to finish and inject its result.
	context.handler.judgeWg.Wait()
	// Drain the leader's own vote from selfInbox and store it via the handler.
	select {
	case event := <-context.selfInbox:
		require.Equal(t, data.ConsensusMessageEvent, event.Type)
		require.Equal(t, data.AnswerClassificationVoteConsensusMessage, event.Message.ConsensusMessageType)
		err := context.handler.HandleAnswerClassificationVote(context.roundKey, event.Message.AnswerClassificationVote)
		require.NoError(t, err)
	default:
		t.Fatal("expected leader vote in selfInbox after judge goroutine completed")
	}
	votes, err := context.roundState.GetAnswerClassificationVotes(context.roundKey)
	require.NoError(t, err)
	require.Len(t, votes, 1)
	return votes[0]
}

func signedClassificationVote(
	t *testing.T,
	template *data.AnswerClassificationVote,
	judgeID string,
	signer signing.MessageSigner,
) *data.AnswerClassificationVote {
	t.Helper()

	vote := *template
	vote.JudgeID = judgeID
	vote.ModelMetadata = "test-model-" + judgeID
	vote.AnswerClassifications = append([]data.AnswerClassificationPerCandidate(nil), template.AnswerClassifications...)
	signClassificationVote(t, &vote, signer)

	return &vote
}

func signClassificationVote(
	t *testing.T,
	vote *data.AnswerClassificationVote,
	signer signing.MessageSigner,
) {
	t.Helper()

	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	require.NoError(t, err)
	signature, err := signer.Sign(voteHash)
	require.NoError(t, err)
	vote.VoteHash = voteHash
	vote.Signature = signature
}

func classificationJudgeIDs(votes []data.AnswerClassificationVote) []string {
	judgeIDs := make([]string, len(votes))
	for index := range votes {
		judgeIDs[index] = votes[index].JudgeID
	}

	return judgeIDs
}

func TestHandleAnswerClassificationCertificateFinalizesCanonicalArtifacts(t *testing.T) {
	t.Parallel()

	leaderContext, certificate := classificationCertificateFixture(t, false)
	firstHandler, firstFinalizer := classificationCertificateReceiver(t, leaderContext, "validator-a")
	secondHandler, secondFinalizer := classificationCertificateReceiver(t, leaderContext, "validator-b")

	require.NoError(t, firstHandler.HandleAnswerClassificationCertificate(leaderContext.roundKey, certificate))
	require.NoError(t, secondHandler.HandleAnswerClassificationCertificate(leaderContext.roundKey, certificate))

	firstBlock, err := firstFinalizer.GetFinalizedBlockInMRTwo(leaderContext.roundKey)
	require.NoError(t, err)
	secondBlock, err := secondFinalizer.GetFinalizedBlockInMRTwo(leaderContext.roundKey)
	require.NoError(t, err)
	require.Equal(t, firstBlock, secondBlock)
	require.Same(t, leaderContext.evidence, firstBlock.AnswerEvidence)
	require.Equal(t, certificate.Transactions, firstBlock.AnswerClassifications)

	for _, transaction := range firstBlock.AnswerClassifications {
		require.Equal(t, data.TransactionAnswerStatusReadyForMiniRoundThree, transaction.Status)
		require.Len(t, transaction.Groups.Correct, 3)
		require.Empty(t, transaction.Groups.Hallucination)
		require.Empty(t, transaction.Groups.Malicious)
		require.Empty(t, transaction.Groups.Wrong)
	}
}

func TestHandleAnswerClassificationCertificateRejectsTamperedResult(t *testing.T) {
	t.Parallel()

	leaderContext, certificate := classificationCertificateFixture(t, false)
	handler, finalizer := classificationCertificateReceiver(t, leaderContext, "validator-a")
	tampered := *certificate
	tampered.Transactions = append([]data.TransactionAnswerClassification(nil), certificate.Transactions...)
	tampered.Transactions[0].Status = data.TransactionAnswerStatusInsufficientCorrectAnswers

	err := handler.HandleAnswerClassificationCertificate(leaderContext.roundKey, &tampered)

	require.ErrorIs(t, err, ErrClassificationCertificateResultMismatch)
	finalized, getErr := finalizer.GetFinalizedBlockInMRTwo(leaderContext.roundKey)
	require.Nil(t, finalized)
	require.Error(t, getErr)
}

func TestHandleAnswerClassificationCertificateVerifiesEmbeddedVoteSignatures(t *testing.T) {
	t.Parallel()

	leaderContext, certificate := classificationCertificateFixture(t, false)
	handler, finalizer := classificationCertificateReceiver(t, leaderContext, "validator-a")
	tampered := *certificate
	tampered.Votes = append([]data.AnswerClassificationVote(nil), certificate.Votes...)
	tampered.Votes[0].Signature = append([]byte(nil), certificate.Votes[0].Signature...)
	tampered.Votes[0].Signature[0] ^= 0xff

	err := handler.HandleAnswerClassificationCertificate(leaderContext.roundKey, &tampered)

	require.ErrorIs(t, err, signing.ErrWrongSignature)
	finalized, getErr := finalizer.GetFinalizedBlockInMRTwo(leaderContext.roundKey)
	require.Nil(t, finalized)
	require.Error(t, getErr)
}

func TestHandleAnswerClassificationCertificateFinalizesInsufficientCorrectStatus(t *testing.T) {
	t.Parallel()

	leaderContext, certificate := classificationCertificateFixture(t, true)
	handler, finalizer := classificationCertificateReceiver(t, leaderContext, "validator-a")

	require.NoError(t, handler.HandleAnswerClassificationCertificate(leaderContext.roundKey, certificate))
	finalized, err := finalizer.GetFinalizedBlockInMRTwo(leaderContext.roundKey)
	require.NoError(t, err)
	for _, transaction := range finalized.AnswerClassifications {
		require.Equal(t, data.TransactionAnswerStatusInsufficientCorrectAnswers, transaction.Status)
		require.Empty(t, transaction.Groups.Correct)
		require.Empty(t, transaction.Groups.Hallucination)
		require.Empty(t, transaction.Groups.Malicious)
		require.Len(t, transaction.Groups.Wrong, 3)
	}
}

func TestHandleAnswerClassificationCertificateRejectsDuplicate(t *testing.T) {
	t.Parallel()

	leaderContext, certificate := classificationCertificateFixture(t, false)
	handler, _ := classificationCertificateReceiver(t, leaderContext, "validator-a")
	require.NoError(t, handler.HandleAnswerClassificationCertificate(leaderContext.roundKey, certificate))

	err := handler.HandleAnswerClassificationCertificate(leaderContext.roundKey, certificate)

	require.ErrorIs(t, err, state.ErrAnswerClassificationCertificateAlreadyExists)
}

func classificationCertificateFixture(
	t *testing.T,
	classifyExternalVotesAsWrong bool,
) (classificationProductionContext, *data.AnswerClassificationCertificate) {
	t.Helper()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	require.NoError(t, context.handler.HandleAnswerEvidence(context.roundKey, context.evidence))
	leaderVote := classificationLeaderVote(t, context)
	for _, judgeID := range []string{"validator-a", "validator-b"} {
		vote := signedClassificationVote(t, leaderVote, judgeID, context.memberSigners[judgeID])
		if classifyExternalVotesAsWrong {
			for index := range vote.AnswerClassifications {
				vote.AnswerClassifications[index].Category = data.AnswerCategoryWrong
			}
			signClassificationVote(t, vote, context.memberSigners[judgeID])
		}
		require.NoError(t, context.handler.HandleAnswerClassificationVote(context.roundKey, vote))
	}

	return context, context.broadcaster.BroadcastAnswerClassificationCertificateMessage.AnswerClassificationCertificate
}

func classificationCertificateReceiver(
	t *testing.T,
	leaderContext classificationProductionContext,
	receiverID string,
) (*miniRoundTwoHandler, *blockFinalizer.FinalizeBlockComponent) {
	t.Helper()

	roundState := state.NewRoundState()
	finalizer := createSeededFinalizer(t, createTestFinalizedBlock())
	handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
		myID:              receiverID,
		signer:            leaderContext.memberSigners[receiverID],
		answerJudge:       &classificationProductionJudge{},
		roundState:        roundState,
		broadcaster:       &testscommon.BroadcasterStub{},
		blockFinalizer:    finalizer,
		validatorRegistry: leaderContext.registry,
	})
	t.Cleanup(func() { handler.judgeWg.Wait() })
	require.NoError(t, handler.HandleAnswerEvidence(leaderContext.roundKey, leaderContext.evidence))

	return handler, finalizer
}

func TestMiniRoundTwoHandler_HandleAnswerClassificationVote(t *testing.T) {
	t.Parallel()

	context := newClassificationProductionContext(t, "leader", "leader", &classificationProductionJudge{})
	err := context.handler.HandleAnswerEvidence(context.roundKey, context.evidence)
	require.NoError(t, err)
	leaderVote := classificationLeaderVote(t, context)

	err = context.handler.HandleAnswerClassificationVote(context.roundKey, leaderVote)
	require.ErrorIs(t, err, state.ErrAnswerClassificationVoteAlreadyExistsForJudge)
	require.ErrorIs(t,
		context.handler.HandleAnswerClassificationVote(context.roundKey, nil),
		ErrNilAnswerClassificationVote,
	)

	wrongRoundVote := *leaderVote
	wrongRoundVote.Round++
	require.ErrorIs(t,
		context.handler.HandleAnswerClassificationVote(context.roundKey, &wrongRoundVote),
		ErrAnswerClassificationVoteMismatch,
	)
}

func TestMiniRoundTwoHandler_HandleAnswerClassificationCertificate(t *testing.T) {
	t.Parallel()

	roundKey := createTestRoundKey()
	roundState := state.NewRoundState()
	handler := createTestMiniRoundTwoHandler(testMiniRoundTwoHandlerArgs{
		roundState: roundState,
		validatorRegistry: &testscommon.ValidatorRegistryStub{
			LeaderID: "leader",
		},
	})
	certificate := plumbingClassificationCertificate(roundKey)

	err := handler.HandleAnswerClassificationCertificate(roundKey, certificate)
	require.ErrorIs(t, err, state.ErrNoAnswerEvidenceForCurrentRoundKey)

	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, nil),
		ErrNilAnswerClassificationCertificate,
	)

	misaligned := plumbingClassificationCertificate(roundKey)
	misaligned.Votes[0].AnswerEvidenceHash = []byte("different-evidence")
	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, misaligned),
		ErrAnswerClassificationCertificateMismatch,
	)

	missingResults := plumbingClassificationCertificate(roundKey)
	missingResults.Transactions = nil
	require.ErrorIs(t,
		handler.HandleAnswerClassificationCertificate(roundKey, missingResults),
		ErrAnswerClassificationCertificateMismatch,
	)
}

func plumbingClassificationVote(roundKey data.RoundKey, judgeID string) *data.AnswerClassificationVote {
	return &data.AnswerClassificationVote{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		JudgeID:            judgeID,
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		AnswerClassifications: []data.AnswerClassificationPerCandidate{
			{Category: data.AnswerCategoryCorrect},
		},
	}
}

func plumbingClassificationCertificate(roundKey data.RoundKey) *data.AnswerClassificationCertificate {
	return &data.AnswerClassificationCertificate{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound,
		SenderID:           "leader",
		CanonicalBlockHash: []byte("canonical-block"),
		AnswerEvidenceHash: []byte("answer-evidence"),
		PromptVersion:      "judge-v1",
		PromptHash:         []byte("prompt-hash"),
		Votes: []data.AnswerClassificationVote{
			*plumbingClassificationVote(roundKey, "judge-a"),
		},
		Transactions: []data.TransactionAnswerClassification{
			{
				TxHash: []byte("tx-a"),
				Status: data.TransactionAnswerStatusInsufficientCorrectAnswers,
			},
		},
	}
}
