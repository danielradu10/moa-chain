package miniround2

import (
	"bytes"
	"reflect"
	"sort"

	"moa-chain/blockprocessing/hashing"
	"moa-chain/consensus/miniround2/classification"
	"moa-chain/data"
	"moa-chain/state"
)

// This file implements the classification half of mini-round two: verified
// answer evidence becomes signed judge votes, then a deterministic certificate.
// Only that verified certificate may finalize the round.

// HandleAnswerEvidence verifies and stores the leader's complete answer
// evidence before invoking the local judge and producing a signed vote. Answer
// evidence alone never finalizes mini-round two.
func (handler *miniRoundTwoHandler) HandleAnswerEvidence(
	roundKey data.RoundKey,
	message *data.AggregatedExecutionResultsMessage,
) error {
	if err := handler.verifyAnswerEvidence(roundKey, message); err != nil {
		return err
	}
	if err := handler.roundState.SetAnswerEvidence(roundKey, message); err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidence failed to store verified evidence", "roundKey", roundKey, "error", err)
		return err
	}
	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		handler.logger.Info("miniround2.HandleAnswerEvidence observer stored evidence without voting", "roundKey", roundKey, "validatorID", handler.myID)
		return nil
	}

	finalizedBlock, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidence failed to load canonical block", "roundKey", roundKey, "error", err)
		return err
	}

	requests, err := classification.BuildAnswerJudgeRequests(&finalizedBlock.Block, message)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidence failed to build judge requests", "roundKey", roundKey, "error", err)
		return err
	}

	answersClassification, err := classification.ExecuteRequests(handler.answerJudge, requests)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidence judge failed", "roundKey", roundKey, "judgeID", handler.myID, "error", err)
		return err
	}

	vote, err := handler.createSignedAnswerClassificationVote(roundKey, message, requests, answersClassification)
	if err != nil {
		handler.logger.Error("miniround2.HandleAnswerEvidence failed to create vote", "roundKey", roundKey, "judgeID", handler.myID, "error", err)
		return err
	}

	return handler.dispatchAnswerClassificationVote(roundKey, vote)
}

// createSignedAnswerClassificationVote validates complete candidate coverage,
// binds the classifications to the exact evidence and prompt, and signs the vote.
func (handler *miniRoundTwoHandler) createSignedAnswerClassificationVote(
	roundKey data.RoundKey,
	evidence *data.AggregatedExecutionResultsMessage,
	requests []classification.TransactionAnswerJudgeRequest,
	classifications []data.AnswerClassificationPerCandidate,
) (*data.AnswerClassificationVote, error) {
	expectedCandidates := judgeRequestCandidateIDs(requests)
	if err := classification.ValidateAnswerClassifications(expectedCandidates, classifications); err != nil {
		return nil, err
	}

	evidenceHash, err := hashing.ComputeAnswerEvidenceHash(evidence)
	if err != nil {
		return nil, err
	}

	vote := &data.AnswerClassificationVote{
		Epoch:                 roundKey.Epoch,
		Round:                 roundKey.Round,
		MiniRound:             roundKey.MiniRound,
		CanonicalBlockHash:    evidence.CanonicalBlockHash,
		AnswerEvidenceHash:    evidenceHash,
		JudgeID:               handler.myID,
		PromptVersion:         classification.AnswerJudgePromptVersion,
		PromptHash:            classification.AnswerJudgePromptHash(),
		ModelMetadata:         handler.judgeModelMetadata,
		AnswerClassifications: classifications,
	}
	if err = handler.signAnswerClassificationVote(vote, evidenceHash); err != nil {
		return nil, err
	}

	return vote, nil
}

// judgeRequestCandidateIDs extracts the protocol identities hidden behind all
// per-transaction model aliases.
func judgeRequestCandidateIDs(requests []classification.TransactionAnswerJudgeRequest) []data.AnswerCandidateID {
	candidates := make([]data.AnswerCandidateID, 0)
	for _, request := range requests {
		for _, reference := range request.Candidates {
			candidates = append(candidates, reference.CandidateID)
		}
	}

	return classification.CanonicalizeAnswerCandidateIDs(candidates)
}

// signAnswerClassificationVote rejects unexpected evidence or prompt identity
// before invoking the node's signer.
func (handler *miniRoundTwoHandler) signAnswerClassificationVote(
	vote *data.AnswerClassificationVote,
	expectedEvidenceHash []byte,
) error {
	if !bytes.Equal(vote.AnswerEvidenceHash, expectedEvidenceHash) {
		return ErrClassificationVoteEvidenceMismatch
	}
	if vote.PromptVersion != classification.AnswerJudgePromptVersion ||
		!bytes.Equal(vote.PromptHash, classification.AnswerJudgePromptHash()) {
		return ErrClassificationVotePromptMismatch
	}

	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	if err != nil {
		return err
	}

	signature, err := handler.signer.Sign(voteHash)
	if err != nil {
		return err
	}

	vote.VoteHash = voteHash
	vote.Signature = signature

	return nil
}

// dispatchAnswerClassificationVote sends validator votes to the leader and
// processes the leader's own vote through the same storage handler.
func (handler *miniRoundTwoHandler) dispatchAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) error {
	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if leaderID == handler.myID {
		return handler.HandleAnswerClassificationVote(roundKey, vote)
	}

	err = handler.broadcaster.SendAnswerClassificationVoteToLeader(&data.ConsensusMessage{
		ConsensusMessageType:     data.AnswerClassificationVoteConsensusMessage,
		AnswerClassificationVote: vote,
	}, leaderID)
	if err != nil {
		handler.logger.Error("miniround2.dispatchAnswerClassificationVote failed to send vote", "roundKey", roundKey, "judgeID", handler.myID, "leaderID", leaderID, "error", err)
	}

	return err
}

// HandleAnswerClassificationVote verifies and stores one judge vote. Once the
// leader reaches classification quorum, it derives and broadcasts the canonical
// classification certificate.
func (handler *miniRoundTwoHandler) HandleAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) error {
	if err := handler.verifyClassificationCollectionLeader(roundKey); err != nil {
		return err
	}

	expectedCandidates, err := handler.verifyAnswerClassificationVote(roundKey, vote)
	if err != nil {
		judgeID := ""
		if vote != nil {
			judgeID = vote.JudgeID
		}

		handler.logger.Error("miniround2.HandleAnswerClassificationVote rejected vote", "roundKey", roundKey, "judgeID", judgeID, "error", err)
		return err
	}

	if handler.roundState.IsAnswerClassificationCertificateSet(roundKey) {
		handler.logger.Info("miniround2.HandleAnswerClassificationVote certificate already produced", "roundKey", roundKey, "judgeID", vote.JudgeID)
		return nil
	}

	if err = handler.roundState.AddAnswerClassificationVote(roundKey, vote); err != nil {
		handler.logger.Error("miniround2.HandleAnswerClassificationVote failed to store vote", "roundKey", roundKey, "judgeID", vote.JudgeID, "error", err)
		return err
	}

	return handler.broadcastClassificationCertificateAtQuorum(roundKey, expectedCandidates)
}

// verifyClassificationCollectionLeader prevents non-leaders from accepting or
// aggregating classification votes.
func (handler *miniRoundTwoHandler) verifyClassificationCollectionLeader(roundKey data.RoundKey) error {
	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}

	if leaderID != handler.myID {
		handler.logger.Error("miniround2.verifyClassificationCollectionLeader local node is not leader", "roundKey", roundKey, "validatorID", handler.myID, "leaderID", leaderID)
		return ErrOnlyLeaderCanCollectVotes
	}

	return nil
}

// verifyAnswerClassificationVote authenticates a judge vote and checks that it
// covers the exact evidence, prompt, and candidates classified by the leader.
func (handler *miniRoundTwoHandler) verifyAnswerClassificationVote(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) ([]data.AnswerCandidateID, error) {
	if vote == nil {
		return nil, ErrNilAnswerClassificationVote
	}
	if !classificationVoteMatchesRound(roundKey, vote) {
		return nil, ErrAnswerClassificationVoteMismatch
	}
	if !handler.validatorRegistry.IsValidatorInConsensusGroup(vote.JudgeID) {
		return nil, ErrValidatorNotPartOfConsensusGroup
	}
	if vote.PromptVersion != classification.AnswerJudgePromptVersion ||
		!bytes.Equal(vote.PromptHash, classification.AnswerJudgePromptHash()) {
		return nil, ErrClassificationVotePromptMismatch
	}

	expectedCandidates, err := handler.classificationCollectionCandidates(roundKey, vote)
	if err != nil {
		return nil, err
	}

	if err = classification.ValidateAnswerClassifications(expectedCandidates, vote.AnswerClassifications); err != nil {
		return nil, err
	}

	if err = handler.verifyAnswerClassificationVoteSignature(vote); err != nil {
		return nil, err
	}

	return expectedCandidates, nil
}

// classificationCollectionCandidates uses the leader's signed local vote as
// the trusted context established from verified answer evidence.
func (handler *miniRoundTwoHandler) classificationCollectionCandidates(
	roundKey data.RoundKey,
	vote *data.AnswerClassificationVote,
) ([]data.AnswerCandidateID, error) {
	votes, err := handler.roundState.GetAnswerClassificationVotes(roundKey)
	if err != nil {
		// The leader's own vote is the first vote and establishes collection context.
		if vote.JudgeID != handler.myID {
			return nil, ErrMissingClassificationCollectionContext
		}

		return classificationCandidateIDs(vote.AnswerClassifications), nil
	}

	for _, storedVote := range votes {
		if storedVote.JudgeID != handler.myID {
			continue
		}
		if !classification.SameVoteContext(*storedVote, *vote) {
			return nil, classification.ErrClassificationVoteContextMismatch
		}

		return classificationCandidateIDs(storedVote.AnswerClassifications), nil
	}

	return nil, ErrMissingClassificationCollectionContext
}

// classificationCandidateIDs extracts candidate identities without trusting the
// categories selected by a judge.
func classificationCandidateIDs(classifications []data.AnswerClassificationPerCandidate) []data.AnswerCandidateID {
	candidates := make([]data.AnswerCandidateID, len(classifications))
	for index := range classifications {
		candidates[index] = classifications[index].CandidateID
	}

	return classification.CanonicalizeAnswerCandidateIDs(candidates)
}

// verifyAnswerClassificationVoteSignature recomputes the signed payload hash
// and verifies it against the judge's registered public key.
func (handler *miniRoundTwoHandler) verifyAnswerClassificationVoteSignature(
	vote *data.AnswerClassificationVote,
) error {
	voteHash, err := hashing.ComputeClassificationVoteHash(vote)
	if err != nil {
		return err
	}
	if !bytes.Equal(voteHash, vote.VoteHash) {
		return ErrClassificationVoteHashMismatch
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(vote.JudgeID)
	if err != nil {
		return err
	}

	return handler.signer.Verify(publicKey, voteHash, vote.Signature)
}

// broadcastClassificationCertificateAtQuorum waits for the configured quorum,
// derives canonical groups, and broadcasts one certificate in judge-ID order.
func (handler *miniRoundTwoHandler) broadcastClassificationCertificateAtQuorum(
	roundKey data.RoundKey,
	expectedCandidates []data.AnswerCandidateID,
) error {
	votePointers, err := handler.roundState.GetAnswerClassificationVotes(roundKey)
	if err != nil {
		return err
	}

	committeeSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}

	quorum := classification.ClassificationQuorum(committeeSize)
	if uint64(len(votePointers)) < quorum {
		handler.logger.Info("miniround2.broadcastClassificationCertificateAtQuorum waiting for votes", "roundKey", roundKey, "numVotes", len(votePointers), "quorum", quorum)
		return nil
	}

	votes := make([]data.AnswerClassificationVote, len(votePointers))
	for index, storedVote := range votePointers {
		votes[index] = *storedVote
	}

	transactions, err := classification.AggregateClassificationVotes(expectedCandidates, votes, committeeSize)
	if err != nil {
		return err
	}

	context := votes[0]
	certificate := &data.AnswerClassificationCertificate{
		Epoch: context.Epoch, Round: context.Round, MiniRound: context.MiniRound,
		SenderID:           handler.myID,
		CanonicalBlockHash: context.CanonicalBlockHash,
		AnswerEvidenceHash: context.AnswerEvidenceHash,
		PromptVersion:      context.PromptVersion,
		PromptHash:         context.PromptHash,
		Votes:              classification.CanonicalizeClassificationVotes(votes),
		Transactions:       transactions,
	}

	validatorsIDs := handler.validatorRegistry.GetValidatorsIDs()
	err = handler.broadcaster.BroadcastAnswerClassificationCertificate(&data.ConsensusMessage{
		ConsensusMessageType:            data.AnswerClassificationCertificateConsensusMessage,
		AnswerClassificationCertificate: certificate,
	}, handler.myID, validatorsIDs)
	if err != nil {
		handler.logger.Error("miniround2.broadcastClassificationCertificateAtQuorum failed to broadcast certificate", "roundKey", roundKey, "error", err)
		return err
	}

	return handler.HandleAnswerClassificationCertificate(roundKey, certificate)
}

// HandleAnswerClassificationCertificate verifies every signed vote, recomputes
// the canonical result, and finalizes mini-round two only when both match.
func (handler *miniRoundTwoHandler) HandleAnswerClassificationCertificate(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) error {
	if certificate == nil {
		handler.logger.Error("miniround2.HandleAnswerClassificationCertificate received nil certificate", "roundKey", roundKey)
		return ErrNilAnswerClassificationCertificate
	}

	if !classificationCertificateMatchesRound(roundKey, certificate) {
		handler.logger.Error(
			"miniround2.HandleAnswerClassificationCertificate certificate envelope mismatch",
			"roundKey", roundKey,
			"senderID", certificate.SenderID,
			"answerEvidenceHash", certificate.AnswerEvidenceHash,
			"promptVersion", certificate.PromptVersion,
		)
		return ErrAnswerClassificationCertificateMismatch
	}

	if handler.roundState.IsAnswerClassificationCertificateSet(roundKey) {
		return state.ErrAnswerClassificationCertificateAlreadyExists
	}

	expectedLeader, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if certificate.SenderID != expectedLeader {
		return ErrMessageNotFromLeader
	}

	evidence, expectedCandidates, err := handler.classificationCertificateEvidence(roundKey, certificate)
	if err != nil {
		return err
	}

	if err = classification.ValidateCanonicalClassificationVotes(certificate.Votes); err != nil {
		return err
	}

	// Validate the leader-provided shape before doing signature work. The result
	// is still untrusted: it must exactly match local aggregation below.
	if err = classification.ValidateCanonicalTransactionClassifications(certificate.Transactions); err != nil {
		return err
	}
	for index := range certificate.Votes {
		if err = handler.verifyCertificateVote(&certificate.Votes[index], certificate, expectedCandidates); err != nil {
			handler.logger.Error("miniround2.HandleAnswerClassificationCertificate rejected vote", "roundKey", roundKey, "judgeID", certificate.Votes[index].JudgeID, "error", err)
			return err
		}
	}

	committeeSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}

	expectedTransactions, err := classification.AggregateClassificationVotes(expectedCandidates, certificate.Votes, committeeSize)
	if err != nil {
		return err
	}

	if !reflect.DeepEqual(expectedTransactions, certificate.Transactions) {
		return ErrClassificationCertificateResultMismatch
	}

	if err = handler.finalizeClassifiedAnswers(roundKey, evidence, expectedTransactions); err != nil {
		return err
	}

	if err = handler.roundState.SetAnswerClassificationCertificate(roundKey, certificate); err != nil {
		return err
	}

	handler.logger.Info("miniround2.HandleAnswerClassificationCertificate finalized classifications", "roundKey", roundKey, "numVotes", len(certificate.Votes), "numTransactions", len(certificate.Transactions))
	return nil
}

// HasVerifiedAnswerEvidence tells the round state machine that the leader has
// broadcast evidence and moved from execution-result collection to judging.
func (handler *miniRoundTwoHandler) HasVerifiedAnswerEvidence(roundKey data.RoundKey) bool {
	_, err := handler.roundState.GetAnswerEvidence(roundKey)

	return err == nil
}

// classificationCertificateEvidence binds the certificate to the locally
// verified evidence and reconstructs its complete candidate set.
func (handler *miniRoundTwoHandler) classificationCertificateEvidence(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) (*data.AggregatedExecutionResultsMessage, []data.AnswerCandidateID, error) {
	evidence, err := handler.roundState.GetAnswerEvidence(roundKey)
	if err != nil {
		return nil, nil, err
	}

	evidenceHash, err := hashing.ComputeAnswerEvidenceHash(evidence)
	if err != nil {
		return nil, nil, err
	}

	if !bytes.Equal(evidenceHash, certificate.AnswerEvidenceHash) ||
		!bytes.Equal(evidence.CanonicalBlockHash, certificate.CanonicalBlockHash) {
		return nil, nil, ErrClassificationVoteEvidenceMismatch
	}

	if certificate.PromptVersion != classification.AnswerJudgePromptVersion ||
		!bytes.Equal(certificate.PromptHash, classification.AnswerJudgePromptHash()) {
		return nil, nil, ErrClassificationVotePromptMismatch
	}

	finalizedBlock, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		return nil, nil, err
	}

	requests, err := classification.BuildAnswerJudgeRequests(&finalizedBlock.Block, evidence)
	if err != nil {
		return nil, nil, err
	}

	return evidence, judgeRequestCandidateIDs(requests), nil
}

// verifyCertificateVote authenticates one vote and checks exact agreement with
// the certificate context and locally reconstructed candidates.
func (handler *miniRoundTwoHandler) verifyCertificateVote(
	vote *data.AnswerClassificationVote,
	certificate *data.AnswerClassificationCertificate,
	expectedCandidates []data.AnswerCandidateID,
) error {
	if !handler.validatorRegistry.IsValidatorInConsensusGroup(vote.JudgeID) {
		return ErrValidatorNotPartOfConsensusGroup
	}
	if !bytes.Equal(vote.CanonicalBlockHash, certificate.CanonicalBlockHash) ||
		!bytes.Equal(vote.AnswerEvidenceHash, certificate.AnswerEvidenceHash) ||
		vote.PromptVersion != certificate.PromptVersion ||
		!bytes.Equal(vote.PromptHash, certificate.PromptHash) {
		return classification.ErrClassificationVoteContextMismatch
	}
	if err := classification.ValidateAnswerClassifications(expectedCandidates, vote.AnswerClassifications); err != nil {
		return err
	}

	return handler.verifyAnswerClassificationVoteSignature(vote)
}

// finalizeClassifiedAnswers stores the complete answer evidence together with
// the counts, groups, and transaction status consumed by mini-round three.
func (handler *miniRoundTwoHandler) finalizeClassifiedAnswers(
	roundKey data.RoundKey,
	evidence *data.AggregatedExecutionResultsMessage,
	transactions []data.TransactionAnswerClassification,
) error {
	finalizedBlock, err := handler.blockFinalizer.GetFinalizedBlockInMROne(data.RoundKey{
		Epoch: roundKey.Epoch, Round: roundKey.Round, MiniRound: roundKey.MiniRound - 1,
	})
	if err != nil {
		return err
	}
	aggregatedResults, err := handler.createFinalizedAggregatedExecutionResults(evidence)
	if err != nil {
		return err
	}

	return handler.blockFinalizer.FinalizeBlockMRTwo(roundKey, &data.BlockOnChain{
		Block:                      finalizedBlock.Block,
		SubdomainsFrequencies:      finalizedBlock.SubdomainsFrequencies,
		AggregatedExecutionResults: aggregatedResults,
		AnswerEvidence:             evidence,
		AnswerClassifications:      transactions,
	})
}

// classificationVoteMatchesRound validates fields shared by every classification vote envelope.
func classificationVoteMatchesRound(roundKey data.RoundKey, vote *data.AnswerClassificationVote) bool {
	return vote.Epoch == roundKey.Epoch &&
		vote.Round == roundKey.Round &&
		vote.MiniRound == roundKey.MiniRound &&
		vote.JudgeID != "" &&
		len(vote.CanonicalBlockHash) > 0 &&
		len(vote.AnswerEvidenceHash) > 0 &&
		vote.PromptVersion != "" &&
		len(vote.PromptHash) > 0 &&
		len(vote.AnswerClassifications) > 0
}

// classificationCertificateMatchesRound verifies that every embedded vote is
// aligned with the certificate's round, answer evidence, and prompt identity.
func classificationCertificateMatchesRound(
	roundKey data.RoundKey,
	certificate *data.AnswerClassificationCertificate,
) bool {
	if certificate.Epoch != roundKey.Epoch ||
		certificate.Round != roundKey.Round ||
		certificate.MiniRound != roundKey.MiniRound ||
		certificate.SenderID == "" ||
		len(certificate.CanonicalBlockHash) == 0 ||
		len(certificate.AnswerEvidenceHash) == 0 ||
		certificate.PromptVersion == "" ||
		len(certificate.PromptHash) == 0 ||
		len(certificate.Votes) == 0 ||
		len(certificate.Transactions) == 0 {
		return false
	}

	for index := range certificate.Votes {
		vote := &certificate.Votes[index]
		if !classificationVoteMatchesRound(roundKey, vote) ||
			!bytes.Equal(vote.CanonicalBlockHash, certificate.CanonicalBlockHash) ||
			!bytes.Equal(vote.AnswerEvidenceHash, certificate.AnswerEvidenceHash) ||
			vote.PromptVersion != certificate.PromptVersion ||
			!bytes.Equal(vote.PromptHash, certificate.PromptHash) {
			return false
		}
	}

	return true
}

func (handler *miniRoundTwoHandler) createFinalizedAggregatedExecutionResults(
	message *data.AggregatedExecutionResultsMessage,
) (data.AggregatedExecutionResults, error) {
	if message == nil || len(message.Answers) == 0 {
		return nil, ErrNilAnswers
	}

	txHashSet := make(map[string]struct{})
	for txHash := range message.Answers[0] {
		txHashSet[txHash] = struct{}{}
	}

	// Maps are intentionally converted to a sorted slice before finalization. This makes the
	// finalized txHash -> answers artifact independent from Go map iteration order.
	txHashes := make([]string, 0, len(txHashSet))
	for txHash := range txHashSet {
		txHashes = append(txHashes, txHash)
	}
	sort.Strings(txHashes)

	aggregatedExecutionResults := make(data.AggregatedExecutionResults, 0, len(txHashes))
	for _, txHash := range txHashes {
		answers := make([]data.TransactionResult, 0, len(message.Answers))
		for _, answerSet := range message.Answers {
			// All validators must have answered the same canonical transaction set; otherwise
			// different nodes could derive different finalized aggregates from the same evidence.
			if len(answerSet) != len(txHashSet) {
				return nil, ErrExecutedPromptsAnswersMismatch
			}

			answer, ok := answerSet[txHash]
			if !ok {
				return nil, ErrExecutedPromptsAnswersMismatch
			}
			if !bytes.Equal(answer.TxHash, []byte(txHash)) {
				return nil, ErrExecutedPromptsAnswersMismatch
			}

			answers = append(answers, answer)
		}

		aggregatedExecutionResults = append(aggregatedExecutionResults, data.AggregatedTransactionExecutionResults{
			TxHash:  []byte(txHash),
			Answers: answers,
		})
	}

	return aggregatedExecutionResults, nil
}
