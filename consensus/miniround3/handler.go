package miniround3

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"sort"

	"moa-chain/agent"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/broadcast"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validators"
)

type miniRoundThreeHandler struct {
	myID string

	roundState        state.RoundState
	broadcaster       broadcast.Broadcaster
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
	blockchainState   state.BlockchainState
	blockFinalizer    blockFinalizer.BlockFinalizer
	synthesisAgent    agent.BatchAgent
	logger            *slog.Logger
}

// MiniRoundThreeHandlerArgs contains the dependencies for the MR3 handler.
type MiniRoundThreeHandlerArgs struct {
	MyID string

	RoundState        state.RoundState
	Broadcaster       broadcast.Broadcaster
	Signer            signing.MessageSigner
	ValidatorRegistry validators.ValidatorRegistry
	BlockchainState   state.BlockchainState
	BlockFinalizer    blockFinalizer.BlockFinalizer
	SynthesisAgent    agent.BatchAgent
	Logger            *slog.Logger
}

// NewMiniRoundThreeHandler creates a new mini-round three handler.
func NewMiniRoundThreeHandler(args MiniRoundThreeHandlerArgs) *miniRoundThreeHandler {
	return &miniRoundThreeHandler{
		myID:              args.MyID,
		roundState:        args.RoundState,
		broadcaster:       args.Broadcaster,
		signer:            args.Signer,
		validatorRegistry: args.ValidatorRegistry,
		blockchainState:   args.BlockchainState,
		blockFinalizer:    args.BlockFinalizer,
		synthesisAgent:    args.SynthesisAgent,
		logger:            args.Logger,
	}
}

// HandleConsensusSelection selects the MR3 committee using the subdomain
// frequencies from the MR1 finalized block (same input MR2 used), keyed on
// the MR3 round key so the two committees can differ.
func (handler *miniRoundThreeHandler) HandleConsensusSelection(key data.RoundKey) (string, error) {
	handler.logger.Info("miniround3.HandleConsensusSelection started", "roundKey", key)

	mr1Key := data.RoundKey{
		Epoch:     key.Epoch,
		Round:     key.Round,
		MiniRound: uint64(data.MiniRoundOne),
	}
	mr1Block, err := handler.blockFinalizer.GetFinalizedBlockInMROne(mr1Key)
	if err != nil {
		handler.logger.Error("miniround3.HandleConsensusSelection failed to get MR1 block", "roundKey", key, "error", err)
		return "", err
	}

	err = handler.validatorRegistry.GenerateConsensusGroupMiniRoundThree(
		handler.blockchainState, key, mr1Block.SubdomainsFrequencies,
	)
	if err != nil {
		handler.logger.Error("miniround3.HandleConsensusSelection failed to generate consensus group", "roundKey", key, "error", err)
		return "", err
	}

	consensusGroup, groupErr := handler.validatorRegistry.ConsensusGroup()
	if groupErr == nil {
		handler.logger.Info("miniround3.HandleConsensusSelection selected consensus group", "roundKey", key, "consensusGroup", consensusGroup)
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		handler.logger.Error("miniround3.HandleConsensusSelection failed to read leader", "roundKey", key, "error", err)
		return "", err
	}

	handler.logger.Info("miniround3.HandleConsensusSelection selected leader", "roundKey", key, "leaderID", leaderID)
	return leaderID, nil
}

// HandleSynthesis is called by the round orchestrator only on the leader.
// It runs synchronously: the event loop blocks during the LLM synthesis call,
// which is safe because no MR3 votes can arrive before the proposal exists.
// The leader stores the proposal locally and broadcasts it; it does not evaluate
// its own proposal or cast a vote.
func (handler *miniRoundThreeHandler) HandleSynthesis(key data.RoundKey) error {
	handler.logger.Info("miniround3.HandleSynthesis started", "roundKey", key, "leaderID", handler.myID)

	mr2Key := data.RoundKey{Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound - 1}
	mr2Block, err := handler.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
	if err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to get MR2 block", "roundKey", key, "error", err)
		return err
	}

	evidenceHash, err := hashing.ComputeAnswerEvidenceHash(mr2Block.AnswerEvidence)
	if err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to hash answer evidence", "roundKey", key, "error", err)
		return err
	}

	eligibleTxs := filterEligibleTransactions(mr2Block)
	handler.logger.Info("miniround3.HandleSynthesis eligible transactions", "roundKey", key, "count", len(eligibleTxs))

	synthesizedAnswers := make([]data.SynthesizedAnswer, 0, len(eligibleTxs))

	if len(eligibleTxs) > 0 {
		requests := make([]agent.SynthesisRequest, 0, len(eligibleTxs))
		for _, tx := range eligibleTxs {
			correctAnswers, extractErr := handler.extractCorrectAnswerTexts(mr2Block, tx.GetTxHash())
			if extractErr != nil {
				handler.logger.Error("miniround3.HandleSynthesis failed to extract correct answers", "roundKey", key, "txHash", string(tx.GetTxHash()), "error", extractErr)
				return extractErr
			}
			requests = append(requests, agent.SynthesisRequest{Tx: tx, CorrectAnswers: correctAnswers})
		}

		results, synthErr := handler.synthesisAgent.SynthesizeBatch(requests)
		if synthErr != nil {
			handler.logger.Error("miniround3.HandleSynthesis synthesis call failed", "roundKey", key, "error", synthErr)
			return synthErr
		}

		for _, r := range results {
			answerHash := sha256.Sum256([]byte(r.Answer))
			synthesizedAnswers = append(synthesizedAnswers, data.SynthesizedAnswer{
				TxHash:     r.TxHash,
				Answer:     r.Answer,
				AnswerHash: answerHash[:],
			})
		}

		sort.Slice(synthesizedAnswers, func(i, j int) bool {
			return bytes.Compare(synthesizedAnswers[i].TxHash, synthesizedAnswers[j].TxHash) < 0
		})
	}

	proposal := &data.ProposedSynthesisMessage{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		SenderID:           handler.myID,
		CanonicalBlockHash: mr2Block.Header.HeaderHash,
		AnswerEvidenceHash: evidenceHash,
		SynthesizedAnswers: synthesizedAnswers,
	}

	blockHash, err := hashing.ComputeSynthesisBlockHash(proposal)
	if err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to compute synthesis block hash", "roundKey", key, "error", err)
		return err
	}

	signature, err := handler.signer.Sign(blockHash)
	if err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to sign proposal", "roundKey", key, "error", err)
		return err
	}

	proposal.SynthesisBlockHash = blockHash
	proposal.Signature = signature

	if err = handler.roundState.SetProposedSynthesis(key, proposal); err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to store proposal", "roundKey", key, "error", err)
		return err
	}

	validatorIDs := handler.validatorRegistry.GetValidatorsIDs()
	if err = handler.broadcaster.BroadcastProposedSynthesis(&data.ConsensusMessage{
		ConsensusMessageType: data.ProposedSynthesisConsensusMessage,
		ProposedSynthesis:    proposal,
	}, handler.myID, validatorIDs); err != nil {
		handler.logger.Error("miniround3.HandleSynthesis failed to broadcast proposal", "roundKey", key, "error", err)
		return err
	}

	handler.logger.Info("miniround3.HandleSynthesis proposal broadcast", "roundKey", key, "numSynthesizedAnswers", len(synthesizedAnswers))
	return nil
}

// HandleProposedSynthesis is called on non-leader committee members when the
// leader's proposal arrives. It verifies the proposal and launches a goroutine
// to evaluate it. The leader never calls this — it stores the proposal directly
// in HandleSynthesis and does not cast a vote.
func (handler *miniRoundThreeHandler) HandleProposedSynthesis(key data.RoundKey, msg *data.ProposedSynthesisMessage) error {
	if msg == nil {
		return ErrNilProposedSynthesis
	}

	if msg.Epoch != key.Epoch || msg.Round != key.Round || msg.MiniRound != key.MiniRound {
		handler.logger.Error("miniround3.HandleProposedSynthesis round key mismatch", "roundKey", key, "msgEpoch", msg.Epoch, "msgRound", msg.Round, "msgMiniRound", msg.MiniRound)
		return ErrSynthesisProposalRoundKeyMismatch
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if msg.SenderID != leaderID {
		handler.logger.Error("miniround3.HandleProposedSynthesis message not from leader", "roundKey", key, "senderID", msg.SenderID, "expectedLeader", leaderID)
		return ErrMessageNotFromLeader
	}

	expectedHash, err := hashing.ComputeSynthesisBlockHash(msg)
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedHash, msg.SynthesisBlockHash) {
		handler.logger.Error("miniround3.HandleProposedSynthesis synthesis block hash mismatch", "roundKey", key)
		return ErrSynthesisBlockHashMismatch
	}

	leaderPublicKey, err := handler.validatorRegistry.GetPublicKey(leaderID)
	if err != nil {
		return err
	}
	if err = handler.signer.Verify(leaderPublicKey, msg.SynthesisBlockHash, msg.Signature); err != nil {
		handler.logger.Error("miniround3.HandleProposedSynthesis signature verification failed", "roundKey", key, "error", err)
		return err
	}

	mr2Key := data.RoundKey{Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound - 1}
	mr2Block, err := handler.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
	if err != nil {
		return err
	}

	if err = verifySynthesisProposalCoverage(msg, mr2Block); err != nil {
		handler.logger.Error("miniround3.HandleProposedSynthesis tx coverage mismatch", "roundKey", key, "error", err)
		return err
	}

	if err = handler.roundState.SetProposedSynthesis(key, msg); err != nil {
		handler.logger.Error("miniround3.HandleProposedSynthesis failed to store proposal", "roundKey", key, "error", err)
		return err
	}

	if !handler.validatorRegistry.IsValidatorInConsensusGroup(handler.myID) {
		handler.logger.Info("miniround3.HandleProposedSynthesis observer stored proposal without voting", "roundKey", key)
		return nil
	}

	// TODO: EvaluateSynthesisBatch is a slow LLM call that blocks the event loop.
	// In theory, AggregatedSynthesisVotes can arrive from the leader while this
	// node is still evaluating (if 2f other validators approve first). With no
	// timeout wiring in this PoC that is acceptable — the proposal is already
	// stored so finalization will proceed correctly once the call returns.
	// A future implementation should run evaluation in a goroutine and process
	// AggregatedSynthesisVotes concurrently.
	return handler.evaluateAndVote(key, msg, mr2Block)
}

// evaluateAndVote calls the agent to evaluate the leader's proposal and, if all
// transactions are approved, builds a signed SynthesisVote and sends it to the
// leader. If any transaction is rejected the validator silently abstains.
func (handler *miniRoundThreeHandler) evaluateAndVote(
	key data.RoundKey,
	proposal *data.ProposedSynthesisMessage,
	mr2Block *data.BlockOnChain,
) error {
	handler.logger.Info("miniround3.evaluateAndVote started", "roundKey", key, "validatorID", handler.myID)

	requests := make([]agent.EvaluateSynthesisRequest, 0, len(proposal.SynthesizedAnswers))
	for _, sa := range proposal.SynthesizedAnswers {
		tx := findTransaction(mr2Block.Body.Transactions, sa.TxHash)
		if tx == nil {
			return fmt.Errorf("miniround3: tx hash %q not found in MR2 block", string(sa.TxHash))
		}

		correctAnswers, err := handler.extractCorrectAnswerTexts(mr2Block, sa.TxHash)
		if err != nil {
			return err
		}

		requests = append(requests, agent.EvaluateSynthesisRequest{
			Tx:                tx,
			CorrectAnswers:    correctAnswers,
			ProposedSynthesis: sa.Answer,
		})
	}

	allApproved := true
	if len(requests) > 0 {
		results, err := handler.synthesisAgent.EvaluateSynthesisBatch(requests)
		if err != nil {
			handler.logger.Error("miniround3.evaluateAndVote evaluation call failed", "roundKey", key, "error", err)
			return err
		}

		for _, r := range results {
			if !r.Approved {
				allApproved = false
				handler.logger.Info("miniround3.evaluateAndVote rejected synthesis for tx", "roundKey", key, "txHash", string(r.TxHash))
				break
			}
		}
	}

	if !allApproved {
		handler.logger.Info("miniround3.evaluateAndVote abstaining from vote", "roundKey", key, "validatorID", handler.myID)
		return nil
	}

	vote := &data.SynthesisVote{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		VoterID:            handler.myID,
		SynthesisBlockHash: proposal.SynthesisBlockHash,
	}

	voteHash, err := hashing.ComputeSynthesisVoteHash(vote)
	if err != nil {
		return err
	}

	signature, err := handler.signer.Sign(voteHash)
	if err != nil {
		return err
	}

	vote.VoteHash = voteHash
	vote.Signature = signature

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}

	handler.logger.Info("miniround3.evaluateAndVote sending vote to leader", "roundKey", key, "validatorID", handler.myID, "leaderID", leaderID)
	return handler.broadcaster.SendSynthesisVoteToLeader(&data.ConsensusMessage{
		ConsensusMessageType: data.SynthesisVoteConsensusMessage,
		SynthesisVote:        vote,
	}, leaderID)
}

// verifySynthesisProposalCoverage checks that the proposal's SynthesizedAnswers
// covers exactly the eligible transactions from the local MR2 block — sorted by
// tx hash, no extras, no omissions.
func verifySynthesisProposalCoverage(msg *data.ProposedSynthesisMessage, mr2Block *data.BlockOnChain) error {
	eligibleHashes := eligibleTxHashes(mr2Block)

	if len(msg.SynthesizedAnswers) != len(eligibleHashes) {
		return ErrSynthesisTxCoverageMismatch
	}

	for i, sa := range msg.SynthesizedAnswers {
		if !bytes.Equal(sa.TxHash, eligibleHashes[i]) {
			return ErrSynthesisTxCoverageMismatch
		}
	}

	return nil
}

// eligibleTxHashes returns the tx hashes of transactions that are eligible for
// MR3 synthesis, sorted for deterministic comparison.
func eligibleTxHashes(mr2Block *data.BlockOnChain) [][]byte {
	hashes := make([][]byte, 0)
	for _, c := range mr2Block.AnswerClassifications {
		if c.Status == data.TransactionAnswerStatusReadyForMiniRoundThree {
			hashes = append(hashes, c.TxHash)
		}
	}

	sort.Slice(hashes, func(i, j int) bool {
		return bytes.Compare(hashes[i], hashes[j]) < 0
	})

	return hashes
}

// filterEligibleTransactions returns the transactions from the MR2 block whose
// classification status is READY_FOR_MINI_ROUND_THREE, sorted by tx hash.
func filterEligibleTransactions(mr2Block *data.BlockOnChain) []data.Transaction {
	eligibleSet := make(map[string]struct{})
	for _, c := range mr2Block.AnswerClassifications {
		if c.Status == data.TransactionAnswerStatusReadyForMiniRoundThree {
			eligibleSet[string(c.TxHash)] = struct{}{}
		}
	}

	eligible := make([]data.Transaction, 0, len(eligibleSet))
	for _, tx := range mr2Block.Body.Transactions {
		if _, ok := eligibleSet[string(tx.GetTxHash())]; ok {
			eligible = append(eligible, tx)
		}
	}

	sort.Slice(eligible, func(i, j int) bool {
		return bytes.Compare(eligible[i].GetTxHash(), eligible[j].GetTxHash()) < 0
	})

	return eligible
}

// extractCorrectAnswerTexts recovers the answer strings for all correct
// candidates of a transaction by joining AnswerClassifications.Groups.Correct
// against the AnswerEvidence signer list.
func (handler *miniRoundThreeHandler) extractCorrectAnswerTexts(
	mr2Block *data.BlockOnChain,
	txHash []byte,
) ([]string, error) {
	var classification *data.TransactionAnswerClassification
	for i := range mr2Block.AnswerClassifications {
		if bytes.Equal(mr2Block.AnswerClassifications[i].TxHash, txHash) {
			classification = &mr2Block.AnswerClassifications[i]
			break
		}
	}
	if classification == nil {
		return nil, fmt.Errorf("miniround3: no classification found for tx hash %q", string(txHash))
	}

	evidence := mr2Block.AnswerEvidence
	signerIndex := make(map[string]int, len(evidence.Signers))
	for i, signerID := range evidence.Signers {
		signerIndex[signerID] = i
	}

	texts := make([]string, 0, len(classification.Groups.Correct))
	for _, candidateID := range classification.Groups.Correct {
		idx, ok := signerIndex[candidateID.ProducerID]
		if !ok {
			return nil, fmt.Errorf("miniround3: producer %q not found in answer evidence signers", candidateID.ProducerID)
		}

		result, ok := evidence.Answers[idx][string(txHash)]
		if !ok {
			return nil, fmt.Errorf("miniround3: tx hash %q not found in answers for producer %q", string(txHash), candidateID.ProducerID)
		}

		texts = append(texts, result.Answer)
	}

	return texts, nil
}

// findTransaction returns the transaction with the given hash from the slice,
// or nil if not found.
func findTransaction(txs []data.Transaction, txHash []byte) data.Transaction {
	for _, tx := range txs {
		if bytes.Equal(tx.GetTxHash(), txHash) {
			return tx
		}
	}
	return nil
}

// HandleSynthesisVote is called only on the leader. It verifies the vote,
// stores it, and at 2f approved votes builds the aggregated certificate,
// finalizes the block locally, and broadcasts to all validators.
// The leader itself does not vote — any vote claiming to come from the leader
// is rejected.
func (handler *miniRoundThreeHandler) HandleSynthesisVote(key data.RoundKey, vote *data.SynthesisVote) error {
	if vote == nil {
		return ErrNilSynthesisVote
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if leaderID != handler.myID {
		return ErrOnlyLeaderCanCollectVotes
	}

	if handler.roundState.IsSynthesisCertificateSet(key) {
		handler.logger.Info("miniround3.HandleSynthesisVote certificate already set, ignoring", "roundKey", key, "voterID", vote.VoterID)
		return nil
	}

	if vote.VoterID == leaderID {
		handler.logger.Error("miniround3.HandleSynthesisVote leader vote rejected", "roundKey", key, "voterID", vote.VoterID)
		return ErrLeaderMustNotVote
	}

	if !handler.validatorRegistry.IsValidatorInConsensusGroup(vote.VoterID) {
		return ErrSynthesisVoterNotInConsensusGroup
	}

	proposal, err := handler.roundState.GetProposedSynthesis(key)
	if err != nil {
		return err
	}
	if !bytes.Equal(vote.SynthesisBlockHash, proposal.SynthesisBlockHash) {
		return ErrSynthesisBlockHashMismatch
	}

	expectedHash, err := hashing.ComputeSynthesisVoteHash(vote)
	if err != nil {
		return err
	}
	if !bytes.Equal(expectedHash, vote.VoteHash) {
		return ErrSynthesisVoteHashMismatch
	}

	publicKey, err := handler.validatorRegistry.GetPublicKey(vote.VoterID)
	if err != nil {
		return err
	}
	if err = handler.signer.Verify(publicKey, vote.VoteHash, vote.Signature); err != nil {
		handler.logger.Error("miniround3.HandleSynthesisVote signature verification failed", "roundKey", key, "voterID", vote.VoterID, "error", err)
		return err
	}

	if err = handler.roundState.AddSynthesisVote(key, vote); err != nil {
		handler.logger.Error("miniround3.HandleSynthesisVote failed to store vote", "roundKey", key, "voterID", vote.VoterID, "error", err)
		return err
	}

	handler.logger.Info("miniround3.HandleSynthesisVote vote stored", "roundKey", key, "voterID", vote.VoterID)
	return handler.buildAndBroadcastAtQuorum(key, proposal)
}

// buildAndBroadcastAtQuorum checks whether 2f votes have been collected. If so
// it builds the aggregated certificate, finalizes the block locally, and
// broadcasts the certificate to all validators.
func (handler *miniRoundThreeHandler) buildAndBroadcastAtQuorum(key data.RoundKey, proposal *data.ProposedSynthesisMessage) error {
	votes, err := handler.roundState.GetSynthesisVotes(key)
	if err != nil {
		return err
	}

	committeeSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}

	quorum := (2 * committeeSize) / 3
	if uint64(len(votes)) < quorum {
		handler.logger.Info("miniround3.buildAndBroadcastAtQuorum waiting for more votes", "roundKey", key, "numVotes", len(votes), "quorum", quorum)
		return nil
	}

	handler.logger.Info("miniround3.buildAndBroadcastAtQuorum quorum reached", "roundKey", key, "numVotes", len(votes), "quorum", quorum)

	signers := make([]string, len(votes))
	voteHashes := make([][]byte, len(votes))
	signatures := make([][]byte, len(votes))
	for i, v := range votes {
		signers[i] = v.VoterID
		voteHashes[i] = v.VoteHash
		signatures[i] = v.Signature
	}

	aggregated := &data.AggregatedSynthesisVotes{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		SenderID:           handler.myID,
		SynthesisBlockHash: proposal.SynthesisBlockHash,
		Signers:            signers,
		VoteHashes:         voteHashes,
		Signatures:         signatures,
	}

	if err = handler.roundState.SetSynthesisCertificate(key, aggregated); err != nil {
		return err
	}

	if err = handler.finalizeBlockMRThree(key, proposal); err != nil {
		return err
	}

	validatorIDs := handler.validatorRegistry.GetValidatorsIDs()
	return handler.broadcaster.BroadcastAggregatedSynthesisVotes(&data.ConsensusMessage{
		ConsensusMessageType:     data.AggregatedSynthesisVotesConsensusMessage,
		AggregatedSynthesisVotes: aggregated,
	}, handler.myID, validatorIDs)
}

// HandleAggregatedSynthesisVotes is called on non-leader validators when the
// leader broadcasts the quorum certificate. It verifies every vote in the
// certificate and finalizes the block.
func (handler *miniRoundThreeHandler) HandleAggregatedSynthesisVotes(key data.RoundKey, msg *data.AggregatedSynthesisVotes) error {
	if msg == nil {
		return ErrNilAggregatedSynthesisVotes
	}

	if msg.Epoch != key.Epoch || msg.Round != key.Round || msg.MiniRound != key.MiniRound {
		return ErrAggregatedSynthesisVotesRoundKeyMismatch
	}

	leaderID, err := handler.validatorRegistry.LeaderOfConsensusGroup()
	if err != nil {
		return err
	}
	if msg.SenderID != leaderID {
		return ErrMessageNotFromLeader
	}

	if handler.roundState.IsSynthesisCertificateSet(key) {
		handler.logger.Info("miniround3.HandleAggregatedSynthesisVotes certificate already set", "roundKey", key)
		return nil
	}

	if len(msg.Signers) != len(msg.VoteHashes) || len(msg.Signers) != len(msg.Signatures) {
		return ErrParallelSliceLengthMismatch
	}

	committeeSize, err := handler.validatorRegistry.ConsensusGroupSize()
	if err != nil {
		return err
	}
	quorum := (2 * committeeSize) / 3
	if uint64(len(msg.Signers)) < quorum {
		return ErrNotEnoughSynthesisVotes
	}

	proposal, err := handler.roundState.GetProposedSynthesis(key)
	if err != nil {
		return err
	}
	if !bytes.Equal(msg.SynthesisBlockHash, proposal.SynthesisBlockHash) {
		return ErrSynthesisBlockHashMismatch
	}

	for i, signerID := range msg.Signers {
		if signerID == leaderID {
			return ErrLeaderMustNotVote
		}
		if !handler.validatorRegistry.IsValidatorInConsensusGroup(signerID) {
			return ErrSynthesisVoterNotInConsensusGroup
		}

		reconstructed := &data.SynthesisVote{
			Epoch:              msg.Epoch,
			Round:              msg.Round,
			MiniRound:          msg.MiniRound,
			VoterID:            signerID,
			SynthesisBlockHash: msg.SynthesisBlockHash,
		}
		expectedHash, hashErr := hashing.ComputeSynthesisVoteHash(reconstructed)
		if hashErr != nil {
			return hashErr
		}
		if !bytes.Equal(expectedHash, msg.VoteHashes[i]) {
			return ErrSynthesisVoteHashMismatch
		}

		publicKey, pkErr := handler.validatorRegistry.GetPublicKey(signerID)
		if pkErr != nil {
			return pkErr
		}
		if verifyErr := handler.signer.Verify(publicKey, msg.VoteHashes[i], msg.Signatures[i]); verifyErr != nil {
			handler.logger.Error("miniround3.HandleAggregatedSynthesisVotes signature verification failed", "roundKey", key, "signerID", signerID, "error", verifyErr)
			return verifyErr
		}
	}

	if err = handler.roundState.SetSynthesisCertificate(key, msg); err != nil {
		return err
	}

	return handler.finalizeBlockMRThree(key, proposal)
}

// finalizeBlockMRThree builds the FinalAnswers slice — one entry per
// transaction — and commits it via the block finalizer. Transactions that were
// eligible for synthesis receive status SYNTHESIZED; all others (NonRelated or
// InsufficientCorrectAnswers) receive status SKIPPED.
func (handler *miniRoundThreeHandler) finalizeBlockMRThree(key data.RoundKey, proposal *data.ProposedSynthesisMessage) error {
	mr2Key := data.RoundKey{Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound - 1}
	mr2Block, err := handler.blockFinalizer.GetFinalizedBlockInMRTwo(mr2Key)
	if err != nil {
		return err
	}

	synthesizedByTxHash := make(map[string]string, len(proposal.SynthesizedAnswers))
	for _, sa := range proposal.SynthesizedAnswers {
		synthesizedByTxHash[string(sa.TxHash)] = sa.Answer
	}

	nonRelatedSet := make(map[string]struct{}, len(mr2Block.NonRelatedTransactionHashes))
	for _, h := range mr2Block.NonRelatedTransactionHashes {
		nonRelatedSet[h] = struct{}{}
	}

	classificationStatus := make(map[string]data.TransactionAnswerStatus, len(mr2Block.AnswerClassifications))
	for _, c := range mr2Block.AnswerClassifications {
		classificationStatus[string(c.TxHash)] = c.Status
	}

	finalAnswers := make([]data.FinalAnswer, 0, len(mr2Block.Body.Transactions))
	for _, tx := range mr2Block.Body.Transactions {
		txHashStr := string(tx.GetTxHash())

		if _, isNonRelated := nonRelatedSet[txHashStr]; isNonRelated {
			finalAnswers = append(finalAnswers, data.FinalAnswer{
				TxHash: tx.GetTxHash(),
				Status: data.FinalAnswerStatusSkipped,
			})
			continue
		}

		status, hasClassification := classificationStatus[txHashStr]
		if !hasClassification || status == data.TransactionAnswerStatusInsufficientCorrectAnswers {
			finalAnswers = append(finalAnswers, data.FinalAnswer{
				TxHash: tx.GetTxHash(),
				Status: data.FinalAnswerStatusSkipped,
			})
			continue
		}

		finalAnswers = append(finalAnswers, data.FinalAnswer{
			TxHash: tx.GetTxHash(),
			Answer: synthesizedByTxHash[txHashStr],
			Status: data.FinalAnswerStatusSynthesized,
		})
	}

	sort.Slice(finalAnswers, func(i, j int) bool {
		return bytes.Compare(finalAnswers[i].TxHash, finalAnswers[j].TxHash) < 0
	})

	handler.logger.Info("miniround3.finalizeBlockMRThree finalizing", "roundKey", key, "numFinalAnswers", len(finalAnswers))

	return handler.blockFinalizer.FinalizeBlockMRThree(key, &data.BlockOnChain{
		Header:                     mr2Block.Header,
		Body:                       mr2Block.Body,
		SubdomainsFrequencies:      mr2Block.SubdomainsFrequencies,
		AggregatedExecutionResults: mr2Block.AggregatedExecutionResults,
		AnswerEvidence:             mr2Block.AnswerEvidence,
		AnswerClassifications:      mr2Block.AnswerClassifications,
		FinalAnswers:               finalAnswers,
	})
}
