package miniround2

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
)

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
			for index := range vote.Assignments {
				vote.Assignments[index].Category = data.AnswerCategoryWrong
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
	require.NoError(t, handler.HandleAnswerEvidence(leaderContext.roundKey, leaderContext.evidence))

	return handler, finalizer
}
