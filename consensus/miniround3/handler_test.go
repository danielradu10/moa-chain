package miniround3

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/agent"
	"moa-chain/blockprocessing/blockFinalizer"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/crypto/signing"
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/testscommon"
	"moa-chain/validators"
)

// ──────────────────────────────────── fixtures ───────────────────────────────

func mr3RoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundThree)}
}

func mr2RoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundTwo)}
}

func mr1RoundKey() data.RoundKey {
	return data.RoundKey{Epoch: 1, Round: 2, MiniRound: uint64(data.MiniRoundOne)}
}

// mr2Block returns a BlockOnChain that looks like a finalized MR2 block.
// Both transactions are READY_FOR_MINI_ROUND_THREE; "validator-1" is the sole
// producer in each correct group.
func mr2Block() *data.BlockOnChain {
	tx1 := &testscommon.TransactionStub{}
	tx1.SetTxHash([]byte("txHash1"))
	tx1.SetPrompt([]byte("prompt one"))
	tx2 := &testscommon.TransactionStub{}
	tx2.SetTxHash([]byte("txHash2"))
	tx2.SetPrompt([]byte("prompt two"))

	return &data.BlockOnChain{
		Header: data.ChainBlockHeader{
			HeaderHash: []byte("mr1-canonical-hash"),
		},
		Body: data.BlockBody{
			Transactions: []data.Transaction{tx1, tx2},
		},
		AnswerEvidence: &data.AggregatedExecutionResultsMessage{
			Signers:         []string{"validator-1"},
			BlockHashes:     [][]byte{[]byte("block-hash-v1")},
			BlockSignatures: [][]byte{[]byte("block-sig-v1")},
			Answers: []data.AnswersTxMessage{
				{
					"txHash1": {TxHash: []byte("txHash1"), Answer: "correct answer one"},
					"txHash2": {TxHash: []byte("txHash2"), Answer: "correct answer two"},
				},
			},
		},
		AnswerClassifications: []data.TransactionAnswerClassification{
			{
				TxHash: []byte("txHash1"),
				Groups: data.CanonicalAnswerGroups{
					Correct: []data.AnswerCandidateID{{ProducerID: "validator-1", TxHash: []byte("txHash1")}},
				},
				Status: data.TransactionAnswerStatusReadyForMiniRoundThree,
			},
			{
				TxHash: []byte("txHash2"),
				Groups: data.CanonicalAnswerGroups{
					Correct: []data.AnswerCandidateID{{ProducerID: "validator-1", TxHash: []byte("txHash2")}},
				},
				Status: data.TransactionAnswerStatusReadyForMiniRoundThree,
			},
		},
	}
}

func newKeyPair(t *testing.T) (pub, priv []byte) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return publicKey, privateKey
}

func seedMR1(t *testing.T, f blockFinalizer.BlockFinalizer) {
	t.Helper()
	require.NoError(t, f.FinalizeBlockMROne(mr1RoundKey(), &data.BlockOnChain{
		SubdomainsFrequencies: data.SubdomainsFrequency{"ml_ai": 1},
	}))
}

func seedMR2(t *testing.T, f blockFinalizer.BlockFinalizer, block *data.BlockOnChain) {
	t.Helper()
	require.NoError(t, f.FinalizeBlockMRTwo(mr2RoundKey(), block))
}

// signedProposal builds a ProposedSynthesisMessage with a computed block hash
// and a valid signature from the given signer, bypassing the synthesis agent.
func signedProposal(
	t *testing.T,
	signer signing.MessageSigner,
	block *data.BlockOnChain,
	key data.RoundKey,
	answers []data.SynthesizedAnswer,
) *data.ProposedSynthesisMessage {
	t.Helper()

	evidenceHash, err := hashing.ComputeAnswerEvidenceHash(block.AnswerEvidence)
	require.NoError(t, err)

	proposal := &data.ProposedSynthesisMessage{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		SenderID:           signer.ID(),
		CanonicalBlockHash: block.Header.HeaderHash,
		AnswerEvidenceHash: evidenceHash,
		SynthesizedAnswers: answers,
	}

	blockHash, err := hashing.ComputeSynthesisBlockHash(proposal)
	require.NoError(t, err)

	sig, err := signer.Sign(blockHash)
	require.NoError(t, err)

	proposal.SynthesisBlockHash = blockHash
	proposal.Signature = sig
	return proposal
}

// testSynthesizedAnswers returns the canonical pair of synthesized answers for
// the two transactions in mr2Block(), sorted by tx hash.
func testSynthesizedAnswers() []data.SynthesizedAnswer {
	h1 := sha256.Sum256([]byte("synthesis one"))
	h2 := sha256.Sum256([]byte("synthesis two"))
	return []data.SynthesizedAnswer{
		{TxHash: []byte("txHash1"), Answer: "synthesis one", AnswerHash: h1[:]},
		{TxHash: []byte("txHash2"), Answer: "synthesis two", AnswerHash: h2[:]},
	}
}

// signedVote builds a SynthesisVote with the correct hash and a valid
// signature from the given signer.
func signedVote(
	t *testing.T,
	signer signing.MessageSigner,
	proposal *data.ProposedSynthesisMessage,
	key data.RoundKey,
) *data.SynthesisVote {
	t.Helper()

	vote := &data.SynthesisVote{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		VoterID:            signer.ID(),
		SynthesisBlockHash: proposal.SynthesisBlockHash,
	}

	voteHash, err := hashing.ComputeSynthesisVoteHash(vote)
	require.NoError(t, err)

	sig, err := signer.Sign(voteHash)
	require.NoError(t, err)

	vote.VoteHash = voteHash
	vote.Signature = sig
	return vote
}

// aggregatedVotes assembles an AggregatedSynthesisVotes from individual votes.
func aggregatedVotes(
	leaderID string,
	key data.RoundKey,
	proposal *data.ProposedSynthesisMessage,
	votes ...*data.SynthesisVote,
) *data.AggregatedSynthesisVotes {
	signers := make([]string, len(votes))
	voteHashes := make([][]byte, len(votes))
	signatures := make([][]byte, len(votes))
	for i, v := range votes {
		signers[i] = v.VoterID
		voteHashes[i] = v.VoteHash
		signatures[i] = v.Signature
	}
	return &data.AggregatedSynthesisVotes{
		Epoch:              key.Epoch,
		Round:              key.Round,
		MiniRound:          key.MiniRound,
		SenderID:           leaderID,
		SynthesisBlockHash: proposal.SynthesisBlockHash,
		Signers:            signers,
		VoteHashes:         voteHashes,
		Signatures:         signatures,
	}
}

type mr3HandlerArgs struct {
	myID              string
	roundState        state.RoundState
	broadcaster       *testscommon.BroadcasterStub
	signer            signing.MessageSigner
	validatorRegistry validators.ValidatorRegistry
	blockFinalizer    blockFinalizer.BlockFinalizer
	synthesisAgent    agent.BatchAgent
}

func newHandler(args mr3HandlerArgs) *miniRoundThreeHandler {
	return &miniRoundThreeHandler{
		myID:              args.myID,
		roundState:        args.roundState,
		broadcaster:       args.broadcaster,
		signer:            args.signer,
		validatorRegistry: args.validatorRegistry,
		blockFinalizer:    args.blockFinalizer,
		synthesisAgent:    args.synthesisAgent,
		logger:            slog.Default(),
	}
}

// allApprovedAgent returns a LabelerStub whose EvaluateSynthesisBatch always
// approves every request.
func allApprovedAgent(synthesisResults []agent.SynthesisResult) *testscommon.LabelerStub {
	return &testscommon.LabelerStub{
		SynthesizeBatchCalled: func(requests []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
			return synthesisResults, nil
		},
		EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
			results := make([]agent.EvaluateSynthesisResult, len(requests))
			for i, r := range requests {
				results[i] = agent.EvaluateSynthesisResult{TxHash: r.Tx.GetTxHash(), Approved: true}
			}
			return results, nil
		},
	}
}

// ─────────────────────── HandleConsensusSelection ───────────────────────────

func TestMiniRoundThreeHandler_HandleConsensusSelection(t *testing.T) {
	t.Parallel()

	t.Run("should return selected leader ID", func(t *testing.T) {
		t.Parallel()

		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR1(t, f)

		handler := newHandler(mr3HandlerArgs{
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
			},
		})

		leaderID, err := handler.HandleConsensusSelection(mr3RoundKey())

		require.NoError(t, err)
		require.Equal(t, "leader", leaderID)
	})

	t.Run("should return error when MR1 block is not found", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			blockFinalizer:    blockFinalizer.NewFinalizeBlockComponent(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{},
		})

		_, err := handler.HandleConsensusSelection(mr3RoundKey())

		require.Equal(t, blockFinalizer.ErrFinalizedBlockNotFound, err)
	})

	t.Run("should propagate GenerateConsensusGroupMiniRoundThree error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("group generation failed")
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR1(t, f)

		handler := newHandler(mr3HandlerArgs{
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				GenerateConsensusGroupMiniRoundThreeErr: expectedErr,
			},
		})

		_, err := handler.HandleConsensusSelection(mr3RoundKey())

		require.Equal(t, expectedErr, err)
	})

	t.Run("should propagate LeaderOfConsensusGroup error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("no leader selected")
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR1(t, f)

		handler := newHandler(mr3HandlerArgs{
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderOfConsensusGroupErr: expectedErr,
			},
		})

		_, err := handler.HandleConsensusSelection(mr3RoundKey())

		require.Equal(t, expectedErr, err)
	})
}

// ──────────────────────────── HandleSynthesis ────────────────────────────────

func TestMiniRoundThreeHandler_HandleSynthesis(t *testing.T) {
	t.Parallel()

	t.Run("should synthesize, store proposal and broadcast", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		_ = leaderPub
		signer := signing.NewSigner("leader", leaderPriv)
		block := mr2Block()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		roundState := state.NewRoundState()
		broadcaster := &testscommon.BroadcasterStub{}
		key := mr3RoundKey()

		synthResults := []agent.SynthesisResult{
			{TxHash: []byte("txHash1"), Answer: "synthesis one"},
			{TxHash: []byte("txHash2"), Answer: "synthesis two"},
		}
		agentStub := allApprovedAgent(synthResults)

		handler := newHandler(mr3HandlerArgs{
			myID:           "leader",
			signer:         signer,
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: f,
			synthesisAgent: agentStub,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				ValidatorsIDs: []string{"leader", "v1", "v2"},
			},
		})

		err := handler.HandleSynthesis(key)

		require.NoError(t, err)

		proposal, getErr := roundState.GetProposedSynthesis(key)
		require.NoError(t, getErr)
		require.NotNil(t, proposal)
		require.Equal(t, "leader", proposal.SenderID)
		require.NotEmpty(t, proposal.SynthesisBlockHash)
		require.NotEmpty(t, proposal.Signature)
		require.Len(t, proposal.SynthesizedAnswers, 2)
		require.Equal(t, []byte("txHash1"), proposal.SynthesizedAnswers[0].TxHash)
		require.Equal(t, "synthesis one", proposal.SynthesizedAnswers[0].Answer)
		require.NotEmpty(t, proposal.SynthesizedAnswers[0].AnswerHash)

		require.NotNil(t, broadcaster.BroadcastProposedSynthesisMessage)
		require.Equal(t, data.ProposedSynthesisConsensusMessage, broadcaster.BroadcastProposedSynthesisMessage.ConsensusMessageType)
		require.Equal(t, "leader", broadcaster.BroadcastProposedSynthesisMyID)
	})

	t.Run("should succeed with no eligible transactions", func(t *testing.T) {
		t.Parallel()

		_, leaderPriv := newKeyPair(t)
		signer := signing.NewSigner("leader", leaderPriv)

		tx1 := &testscommon.TransactionStub{}
		tx1.SetTxHash([]byte("txHash1"))
		skippedBlock := &data.BlockOnChain{
			Header: data.ChainBlockHeader{HeaderHash: []byte("mr1-hash")},
			Body:   data.BlockBody{Transactions: []data.Transaction{tx1}},
			AnswerEvidence: &data.AggregatedExecutionResultsMessage{
				Signers:         []string{"validator-1"},
				BlockHashes:     [][]byte{[]byte("block-hash-v1")},
				BlockSignatures: [][]byte{[]byte("block-sig-v1")},
				Answers:         []data.AnswersTxMessage{{}},
			},
			AnswerClassifications: []data.TransactionAnswerClassification{
				{
					TxHash: []byte("txHash1"),
					Status: data.TransactionAnswerStatusInsufficientCorrectAnswers,
				},
			},
		}

		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, skippedBlock)
		broadcaster := &testscommon.BroadcasterStub{}

		handler := newHandler(mr3HandlerArgs{
			myID:           "leader",
			signer:         signer,
			roundState:     state.NewRoundState(),
			broadcaster:    broadcaster,
			blockFinalizer: f,
			synthesisAgent: &testscommon.LabelerStub{},
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				ValidatorsIDs: []string{"leader", "v1"},
			},
		})

		err := handler.HandleSynthesis(mr3RoundKey())

		require.NoError(t, err)
		require.NotNil(t, broadcaster.BroadcastProposedSynthesisMessage)
		require.Empty(t, broadcaster.BroadcastProposedSynthesisMessage.ProposedSynthesis.SynthesizedAnswers)
	})

	t.Run("should return error when MR2 block is not found", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			myID:              "leader",
			blockFinalizer:    blockFinalizer.NewFinalizeBlockComponent(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{},
		})

		err := handler.HandleSynthesis(mr3RoundKey())

		require.Equal(t, blockFinalizer.ErrFinalizedBlockNotFound, err)
	})

	t.Run("should propagate SynthesizeBatch error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("synthesis agent error")
		_, leaderPriv := newKeyPair(t)
		block := mr2Block()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)

		handler := newHandler(mr3HandlerArgs{
			myID:           "leader",
			signer:         signing.NewSigner("leader", leaderPriv),
			roundState:     state.NewRoundState(),
			blockFinalizer: f,
			synthesisAgent: &testscommon.LabelerStub{
				SynthesizeBatchCalled: func(_ []agent.SynthesisRequest) ([]agent.SynthesisResult, error) {
					return nil, expectedErr
				},
			},
			validatorRegistry: &testscommon.ValidatorRegistryStub{},
		})

		err := handler.HandleSynthesis(mr3RoundKey())

		require.Equal(t, expectedErr, err)
	})

	t.Run("should propagate BroadcastProposedSynthesis error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("broadcast error")
		_, leaderPriv := newKeyPair(t)
		block := mr2Block()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)

		synthResults := []agent.SynthesisResult{
			{TxHash: []byte("txHash1"), Answer: "synthesis one"},
			{TxHash: []byte("txHash2"), Answer: "synthesis two"},
		}

		handler := newHandler(mr3HandlerArgs{
			myID:           "leader",
			signer:         signing.NewSigner("leader", leaderPriv),
			roundState:     state.NewRoundState(),
			blockFinalizer: f,
			synthesisAgent: allApprovedAgent(synthResults),
			broadcaster: &testscommon.BroadcasterStub{
				BroadcastProposedSynthesisErr: expectedErr,
			},
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				ValidatorsIDs: []string{"leader", "v1"},
			},
		})

		err := handler.HandleSynthesis(mr3RoundKey())

		require.Equal(t, expectedErr, err)
	})
}

// ─────────────────────── HandleProposedSynthesis ────────────────────────────

func TestMiniRoundThreeHandler_HandleProposedSynthesis(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilProposedSynthesis for nil message", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{},
		})

		err := handler.HandleProposedSynthesis(mr3RoundKey(), nil)

		require.Equal(t, ErrNilProposedSynthesis, err)
	})

	t.Run("should return ErrSynthesisProposalRoundKeyMismatch when round key does not match", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{},
		})
		msg := &data.ProposedSynthesisMessage{Epoch: 99, Round: 99, MiniRound: 99}

		err := handler.HandleProposedSynthesis(mr3RoundKey(), msg)

		require.Equal(t, ErrSynthesisProposalRoundKeyMismatch, err)
	})

	t.Run("should return ErrMessageNotFromLeader when sender is not the elected leader", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})
		msg := &data.ProposedSynthesisMessage{
			Epoch:     key.Epoch,
			Round:     key.Round,
			MiniRound: key.MiniRound,
			SenderID:  "impostor",
		}

		err := handler.HandleProposedSynthesis(key, msg)

		require.Equal(t, ErrMessageNotFromLeader, err)
	})

	t.Run("should return ErrSynthesisBlockHashMismatch when block hash is tampered", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		block := mr2Block()
		key := mr3RoundKey()
		proposal := signedProposal(t, leaderSigner, block, key, testSynthesizedAnswers())
		proposal.SynthesisBlockHash = []byte("tampered-hash")

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
			},
		})

		err := handler.HandleProposedSynthesis(key, proposal)

		require.Equal(t, ErrSynthesisBlockHashMismatch, err)
	})

	t.Run("should return signature error when proposal signature is invalid", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		_, v1Priv := newKeyPair(t)
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		_, wrongPriv := newKeyPair(t)
		block := mr2Block()
		key := mr3RoundKey()

		proposal := signedProposal(t, leaderSigner, block, key, testSynthesizedAnswers())
		// sign with a different key so the signature is invalid for leaderPub
		wrongSigner := signing.NewSigner("leader", wrongPriv)
		wrongSig, err := wrongSigner.Sign(proposal.SynthesisBlockHash)
		require.NoError(t, err)
		proposal.Signature = wrongSig

		handler := newHandler(mr3HandlerArgs{
			signer: signing.NewSigner("v1", v1Priv),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
			},
		})

		err = handler.HandleProposedSynthesis(key, proposal)

		require.Equal(t, signing.ErrWrongSignature, err)
	})

	t.Run("should return ErrSynthesisTxCoverageMismatch when proposal omits eligible txs", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		_, v1Priv := newKeyPair(t)
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		block := mr2Block()
		key := mr3RoundKey()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)

		// proposal covers only one of the two eligible transactions
		partialAnswers := []data.SynthesizedAnswer{
			testSynthesizedAnswers()[0],
		}
		proposal := signedProposal(t, leaderSigner, block, key, partialAnswers)

		handler := newHandler(mr3HandlerArgs{
			signer:         signing.NewSigner("v1", v1Priv),
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
			},
		})

		err := handler.HandleProposedSynthesis(key, proposal)

		require.Equal(t, ErrSynthesisTxCoverageMismatch, err)
	})

	t.Run("should store proposal and abstain when validator is not in consensus group", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		_, observerPriv := newKeyPair(t)
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		block := mr2Block()
		key := mr3RoundKey()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		roundState := state.NewRoundState()
		broadcaster := &testscommon.BroadcasterStub{}

		proposal := signedProposal(t, leaderSigner, block, key, testSynthesizedAnswers())

		handler := newHandler(mr3HandlerArgs{
			myID:           "observer",
			signer:         signing.NewSigner("observer", observerPriv),
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
				ConsensusValidators:     map[string]bool{"observer": false},
			},
		})

		err := handler.HandleProposedSynthesis(key, proposal)

		require.NoError(t, err)
		stored, getErr := roundState.GetProposedSynthesis(key)
		require.NoError(t, getErr)
		require.Same(t, proposal, stored)
		require.Nil(t, broadcaster.SentSynthesisVoteMessage)
	})

	t.Run("should abstain when evaluation rejects any answer", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		_, v1Priv := newKeyPair(t)
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		v1Signer := signing.NewSigner("v1", v1Priv)
		block := mr2Block()
		key := mr3RoundKey()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		broadcaster := &testscommon.BroadcasterStub{}

		proposal := signedProposal(t, leaderSigner, block, key, testSynthesizedAnswers())

		handler := newHandler(mr3HandlerArgs{
			myID:        "v1",
			signer:      v1Signer,
			roundState:  state.NewRoundState(),
			broadcaster: broadcaster,
			blockFinalizer: f,
			synthesisAgent: &testscommon.LabelerStub{
				EvaluateSynthesisBatchCalled: func(requests []agent.EvaluateSynthesisRequest) ([]agent.EvaluateSynthesisResult, error) {
					results := make([]agent.EvaluateSynthesisResult, len(requests))
					for i, r := range requests {
						results[i] = agent.EvaluateSynthesisResult{TxHash: r.Tx.GetTxHash(), Approved: false}
					}
					return results, nil
				},
			},
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
				ConsensusValidators:     map[string]bool{"v1": true},
			},
		})

		err := handler.HandleProposedSynthesis(key, proposal)

		require.NoError(t, err)
		require.Nil(t, broadcaster.SentSynthesisVoteMessage)
	})

	t.Run("should store proposal and send vote when evaluation approves all answers", func(t *testing.T) {
		t.Parallel()

		leaderPub, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		_ = v1Pub
		leaderSigner := signing.NewSigner("leader", leaderPriv)
		v1Signer := signing.NewSigner("v1", v1Priv)
		block := mr2Block()
		key := mr3RoundKey()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		roundState := state.NewRoundState()
		broadcaster := &testscommon.BroadcasterStub{}

		proposal := signedProposal(t, leaderSigner, block, key, testSynthesizedAnswers())

		synthResults := []agent.SynthesisResult{
			{TxHash: []byte("txHash1"), Answer: "synthesis one"},
			{TxHash: []byte("txHash2"), Answer: "synthesis two"},
		}

		handler := newHandler(mr3HandlerArgs{
			myID:           "v1",
			signer:         v1Signer,
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: f,
			synthesisAgent: allApprovedAgent(synthResults),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				PublicKeysByValidatorID: map[string][]byte{"leader": leaderPub},
				ConsensusValidators:     map[string]bool{"v1": true},
			},
		})

		err := handler.HandleProposedSynthesis(key, proposal)

		require.NoError(t, err)
		stored, getErr := roundState.GetProposedSynthesis(key)
		require.NoError(t, getErr)
		require.Same(t, proposal, stored)
		require.NotNil(t, broadcaster.SentSynthesisVoteMessage)
		require.Equal(t, data.SynthesisVoteConsensusMessage, broadcaster.SentSynthesisVoteMessage.ConsensusMessageType)
		require.Equal(t, "leader", broadcaster.SentSynthesisVoteLeaderID)
		vote := broadcaster.SentSynthesisVoteMessage.SynthesisVote
		require.Equal(t, "v1", vote.VoterID)
		require.NotEmpty(t, vote.VoteHash)
		require.NotEmpty(t, vote.Signature)
	})
}

// ──────────────────────── HandleSynthesisVote ───────────────────────────────

func TestMiniRoundThreeHandler_HandleSynthesisVote(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilSynthesisVote for nil vote", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})

		err := handler.HandleSynthesisVote(mr3RoundKey(), nil)

		require.Equal(t, ErrNilSynthesisVote, err)
	})

	t.Run("should return ErrOnlyLeaderCanCollectVotes when called on a non-leader", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			myID: "v1",
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})

		err := handler.HandleSynthesisVote(mr3RoundKey(), &data.SynthesisVote{VoterID: "v2"})

		require.Equal(t, ErrOnlyLeaderCanCollectVotes, err)
	})

	t.Run("should silently ignore vote when certificate is already set", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		require.NoError(t, roundState.SetSynthesisCertificate(key, &data.AggregatedSynthesisVotes{}))

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})

		err := handler.HandleSynthesisVote(key, &data.SynthesisVote{VoterID: "v1"})

		require.NoError(t, err)
	})

	t.Run("should return ErrLeaderMustNotVote when leader tries to vote", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			roundState: state.NewRoundState(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})

		err := handler.HandleSynthesisVote(mr3RoundKey(), &data.SynthesisVote{VoterID: "leader"})

		require.Equal(t, ErrLeaderMustNotVote, err)
	})

	t.Run("should return ErrSynthesisVoterNotInConsensusGroup when voter is outside committee", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			roundState: state.NewRoundState(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:            "leader",
				ConsensusValidators: map[string]bool{"v1": false},
			},
		})

		err := handler.HandleSynthesisVote(mr3RoundKey(), &data.SynthesisVote{VoterID: "v1"})

		require.Equal(t, ErrSynthesisVoterNotInConsensusGroup, err)
	})

	t.Run("should return ErrSynthesisBlockHashMismatch when vote targets a different proposal", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:            "leader",
				ConsensusValidators: map[string]bool{"v1": true},
			},
		})

		err := handler.HandleSynthesisVote(key, &data.SynthesisVote{
			VoterID:            "v1",
			SynthesisBlockHash: []byte("wrong-hash"),
		})

		require.Equal(t, ErrSynthesisBlockHashMismatch, err)
	})

	t.Run("should return ErrSynthesisVoteHashMismatch when vote hash is tampered", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))
		vote := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote.VoteHash = []byte("tampered-vote-hash")

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusValidators:     map[string]bool{"v1": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub},
			},
		})

		err := handler.HandleSynthesisVote(key, vote)

		require.Equal(t, ErrSynthesisVoteHashMismatch, err)
	})

	t.Run("should return signature error when vote signature is invalid", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		_, wrongPriv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))
		vote := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		// replace signature with one from a different key
		wrongSig, err := signing.NewSigner("v1", wrongPriv).Sign(vote.VoteHash)
		require.NoError(t, err)
		vote.Signature = wrongSig

		handler := newHandler(mr3HandlerArgs{
			myID:       "leader",
			signer:     signing.NewSigner("leader", leaderPriv),
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusValidators:     map[string]bool{"v1": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub},
			},
		})

		err = handler.HandleSynthesisVote(key, vote)

		require.Equal(t, signing.ErrWrongSignature, err)
	})

	t.Run("should store vote but not broadcast when quorum is not reached", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))
		vote := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		broadcaster := &testscommon.BroadcasterStub{}

		// committee size 3 → quorum (2*3)/3 = 2; one vote is below quorum
		handler := newHandler(mr3HandlerArgs{
			myID:        "leader",
			signer:      signing.NewSigner("leader", leaderPriv),
			roundState:  roundState,
			broadcaster: broadcaster,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusValidators:     map[string]bool{"v1": true},
				ConsensusGroupSizeValue: 3,
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub},
			},
		})

		err := handler.HandleSynthesisVote(key, vote)

		require.NoError(t, err)
		require.Nil(t, broadcaster.BroadcastAggregatedSynthesisVotesMessage)
	})

	t.Run("should finalize block and broadcast at quorum", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		v2Pub, v2Priv := newKeyPair(t)
		block := mr2Block()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		vote1 := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote2 := signedVote(t, signing.NewSigner("v2", v2Priv), proposal, key)
		broadcaster := &testscommon.BroadcasterStub{}

		// committee size 3 → quorum 2; v1 + v2 reach quorum
		handler := newHandler(mr3HandlerArgs{
			myID:           "leader",
			signer:         signing.NewSigner("leader", leaderPriv),
			roundState:     roundState,
			broadcaster:    broadcaster,
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID: "leader",
				ConsensusValidators: map[string]bool{
					"v1": true,
					"v2": true,
				},
				ConsensusGroupSizeValue: 3,
				PublicKeysByValidatorID: map[string][]byte{
					"v1": v1Pub,
					"v2": v2Pub,
				},
				ValidatorsIDs: []string{"leader", "v1", "v2"},
			},
		})

		require.NoError(t, handler.HandleSynthesisVote(key, vote1))
		require.Nil(t, broadcaster.BroadcastAggregatedSynthesisVotesMessage)

		require.NoError(t, handler.HandleSynthesisVote(key, vote2))

		require.NotNil(t, broadcaster.BroadcastAggregatedSynthesisVotesMessage)
		require.Equal(t, data.AggregatedSynthesisVotesConsensusMessage, broadcaster.BroadcastAggregatedSynthesisVotesMessage.ConsensusMessageType)
		agg := broadcaster.BroadcastAggregatedSynthesisVotesMessage.AggregatedSynthesisVotes
		require.Len(t, agg.Signers, 2)

		finalizedBlock, getErr := f.GetFinalizedBlockInMRThree(key)
		require.NoError(t, getErr)
		require.Len(t, finalizedBlock.FinalAnswers, 2)
		require.Equal(t, data.FinalAnswerStatusSynthesized, finalizedBlock.FinalAnswers[0].Status)
		require.Equal(t, data.FinalAnswerStatusSynthesized, finalizedBlock.FinalAnswers[1].Status)
	})
}

// ──────────────────── HandleAggregatedSynthesisVotes ────────────────────────

func TestMiniRoundThreeHandler_HandleAggregatedSynthesisVotes(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilAggregatedSynthesisVotes for nil message", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})

		err := handler.HandleAggregatedSynthesisVotes(mr3RoundKey(), nil)

		require.Equal(t, ErrNilAggregatedSynthesisVotes, err)
	})

	t.Run("should return ErrAggregatedSynthesisVotesRoundKeyMismatch when round key does not match", func(t *testing.T) {
		t.Parallel()

		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})
		msg := &data.AggregatedSynthesisVotes{Epoch: 99, Round: 99, MiniRound: 99, SenderID: "leader"}

		err := handler.HandleAggregatedSynthesisVotes(mr3RoundKey(), msg)

		require.Equal(t, ErrAggregatedSynthesisVotesRoundKeyMismatch, err)
	})

	t.Run("should return ErrMessageNotFromLeader when sender is not the leader", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		handler := newHandler(mr3HandlerArgs{
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})
		msg := &data.AggregatedSynthesisVotes{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SenderID: "impostor",
		}

		err := handler.HandleAggregatedSynthesisVotes(key, msg)

		require.Equal(t, ErrMessageNotFromLeader, err)
	})

	t.Run("should silently ignore when certificate is already set", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		require.NoError(t, roundState.SetSynthesisCertificate(key, &data.AggregatedSynthesisVotes{}))

		handler := newHandler(mr3HandlerArgs{
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})
		msg := &data.AggregatedSynthesisVotes{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SenderID: "leader",
		}

		err := handler.HandleAggregatedSynthesisVotes(key, msg)

		require.NoError(t, err)
	})

	t.Run("should return ErrParallelSliceLengthMismatch when slices are misaligned", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		handler := newHandler(mr3HandlerArgs{
			roundState: state.NewRoundState(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{LeaderID: "leader"},
		})
		msg := &data.AggregatedSynthesisVotes{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SenderID:   "leader",
			Signers:    []string{"v1"},
			VoteHashes: [][]byte{[]byte("hash")},
			Signatures: nil,
		}

		err := handler.HandleAggregatedSynthesisVotes(key, msg)

		require.Equal(t, ErrParallelSliceLengthMismatch, err)
	})

	t.Run("should return ErrNotEnoughSynthesisVotes when below quorum", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		handler := newHandler(mr3HandlerArgs{
			roundState: state.NewRoundState(),
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
			},
		})
		// quorum = (2*3)/3 = 2; only 1 signer is not enough
		msg := &data.AggregatedSynthesisVotes{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SenderID:   "leader",
			Signers:    []string{"v1"},
			VoteHashes: [][]byte{[]byte("hash")},
			Signatures: [][]byte{[]byte("sig")},
		}

		err := handler.HandleAggregatedSynthesisVotes(key, msg)

		require.Equal(t, ErrNotEnoughSynthesisVotes, err)
	})

	t.Run("should return ErrSynthesisBlockHashMismatch when certificate hash differs from stored proposal", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		v2Pub, v2Priv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		vote1 := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote2 := signedVote(t, signing.NewSigner("v2", v2Priv), proposal, key)
		agg := aggregatedVotes("leader", key, proposal, vote1, vote2)
		agg.SynthesisBlockHash = []byte("wrong-hash")

		handler := newHandler(mr3HandlerArgs{
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
				ConsensusValidators:     map[string]bool{"v1": true, "v2": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub, "v2": v2Pub},
			},
		})

		err := handler.HandleAggregatedSynthesisVotes(key, agg)

		require.Equal(t, ErrSynthesisBlockHashMismatch, err)
	})

	t.Run("should return ErrLeaderMustNotVote when leader appears in signers", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		// build a fake aggregated cert where the "leader" is listed as signer
		agg := &data.AggregatedSynthesisVotes{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SenderID:           "leader",
			SynthesisBlockHash: proposal.SynthesisBlockHash,
			Signers:            []string{"leader", "v1"},
			VoteHashes:         [][]byte{[]byte("h1"), []byte("h2")},
			Signatures:         [][]byte{[]byte("s1"), []byte("s2")},
		}

		handler := newHandler(mr3HandlerArgs{
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
			},
		})

		err := handler.HandleAggregatedSynthesisVotes(key, agg)

		require.Equal(t, ErrLeaderMustNotVote, err)
	})

	t.Run("should return ErrSynthesisVoteHashMismatch when a signer's vote hash is wrong", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		v2Pub, v2Priv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		vote1 := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote2 := signedVote(t, signing.NewSigner("v2", v2Priv), proposal, key)
		agg := aggregatedVotes("leader", key, proposal, vote1, vote2)
		agg.VoteHashes[0] = []byte("tampered-hash")

		handler := newHandler(mr3HandlerArgs{
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
				ConsensusValidators:     map[string]bool{"v1": true, "v2": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub, "v2": v2Pub},
			},
		})

		err := handler.HandleAggregatedSynthesisVotes(key, agg)

		require.Equal(t, ErrSynthesisVoteHashMismatch, err)
	})

	t.Run("should return signature error when a signer's signature is invalid", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		_, v3Priv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		v2Pub, v2Priv := newKeyPair(t)
		_, wrongPriv := newKeyPair(t)
		block := mr2Block()
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		vote1 := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote2 := signedVote(t, signing.NewSigner("v2", v2Priv), proposal, key)
		agg := aggregatedVotes("leader", key, proposal, vote1, vote2)
		// replace v1's signature with one from a different key
		wrongSig, err := signing.NewSigner("v1", wrongPriv).Sign(vote1.VoteHash)
		require.NoError(t, err)
		agg.Signatures[0] = wrongSig

		handler := newHandler(mr3HandlerArgs{
			myID:       "v3",
			signer:     signing.NewSigner("v3", v3Priv),
			roundState: roundState,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
				ConsensusValidators:     map[string]bool{"v1": true, "v2": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub, "v2": v2Pub},
			},
		})

		err = handler.HandleAggregatedSynthesisVotes(key, agg)

		require.Equal(t, signing.ErrWrongSignature, err)
	})

	t.Run("should set certificate and finalize block when all votes are valid", func(t *testing.T) {
		t.Parallel()

		key := mr3RoundKey()
		roundState := state.NewRoundState()
		_, leaderPriv := newKeyPair(t)
		_, v3Priv := newKeyPair(t)
		v1Pub, v1Priv := newKeyPair(t)
		v2Pub, v2Priv := newKeyPair(t)
		block := mr2Block()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		proposal := signedProposal(t, signing.NewSigner("leader", leaderPriv), block, key, testSynthesizedAnswers())
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		vote1 := signedVote(t, signing.NewSigner("v1", v1Priv), proposal, key)
		vote2 := signedVote(t, signing.NewSigner("v2", v2Priv), proposal, key)
		agg := aggregatedVotes("leader", key, proposal, vote1, vote2)

		handler := newHandler(mr3HandlerArgs{
			myID:           "v3",
			signer:         signing.NewSigner("v3", v3Priv),
			roundState:     roundState,
			blockFinalizer: f,
			validatorRegistry: &testscommon.ValidatorRegistryStub{
				LeaderID:                "leader",
				ConsensusGroupSizeValue: 3,
				ConsensusValidators:     map[string]bool{"v1": true, "v2": true},
				PublicKeysByValidatorID: map[string][]byte{"v1": v1Pub, "v2": v2Pub},
			},
		})

		err := handler.HandleAggregatedSynthesisVotes(key, agg)

		require.NoError(t, err)
		require.True(t, roundState.IsSynthesisCertificateSet(key))

		finalizedBlock, getErr := f.GetFinalizedBlockInMRThree(key)
		require.NoError(t, getErr)
		require.Len(t, finalizedBlock.FinalAnswers, 2)
		require.Equal(t, data.FinalAnswerStatusSynthesized, finalizedBlock.FinalAnswers[0].Status)
		require.Equal(t, "synthesis one", finalizedBlock.FinalAnswers[0].Answer)
	})
}

// ─────────────────────── finalizeBlockMRThree ───────────────────────────────

func TestMiniRoundThreeHandler_finalizeBlockMRThree(t *testing.T) {
	t.Parallel()

	t.Run("should mark READY transactions as SYNTHESIZED and others as SKIPPED", func(t *testing.T) {
		t.Parallel()

		txReady := &testscommon.TransactionStub{}
		txReady.SetTxHash([]byte("txReady"))
		txInsufficient := &testscommon.TransactionStub{}
		txInsufficient.SetTxHash([]byte("txInsufficient"))
		txNonRelated := &testscommon.TransactionStub{}
		txNonRelated.SetTxHash([]byte("txNonRelated"))

		block := &data.BlockOnChain{
			Header: data.ChainBlockHeader{HeaderHash: []byte("hash")},
			Body: data.BlockBody{
				Transactions: []data.Transaction{txReady, txInsufficient, txNonRelated},
			},
			AnswerEvidence: &data.AggregatedExecutionResultsMessage{
				Signers:         []string{"validator-1"},
				BlockHashes:     [][]byte{[]byte("block-hash-v1")},
				BlockSignatures: [][]byte{[]byte("block-sig-v1")},
				Answers:         []data.AnswersTxMessage{{}},
			},
			NonRelatedTransactionHashes: []string{"txNonRelated"},
			AnswerClassifications: []data.TransactionAnswerClassification{
				{
					TxHash: []byte("txReady"),
					Status: data.TransactionAnswerStatusReadyForMiniRoundThree,
				},
				{
					TxHash: []byte("txInsufficient"),
					Status: data.TransactionAnswerStatusInsufficientCorrectAnswers,
				},
			},
		}

		key := mr3RoundKey()
		f := blockFinalizer.NewFinalizeBlockComponent()
		seedMR2(t, f, block)
		roundState := state.NewRoundState()
		proposal := &data.ProposedSynthesisMessage{
			Epoch: key.Epoch, Round: key.Round, MiniRound: key.MiniRound,
			SynthesizedAnswers: []data.SynthesizedAnswer{
				{TxHash: []byte("txReady"), Answer: "synthesized answer"},
			},
			SynthesisBlockHash: []byte("dummy"),
		}
		require.NoError(t, roundState.SetProposedSynthesis(key, proposal))

		handler := newHandler(mr3HandlerArgs{
			roundState:     roundState,
			blockFinalizer: f,
		})

		err := handler.finalizeBlockMRThree(key, proposal)

		require.NoError(t, err)

		finalizedBlock, getErr := f.GetFinalizedBlockInMRThree(key)
		require.NoError(t, getErr)
		require.Len(t, finalizedBlock.FinalAnswers, 3)

		byHash := make(map[string]data.FinalAnswer, 3)
		for _, fa := range finalizedBlock.FinalAnswers {
			byHash[string(fa.TxHash)] = fa
		}

		require.Equal(t, data.FinalAnswerStatusSynthesized, byHash["txReady"].Status)
		require.Equal(t, "synthesized answer", byHash["txReady"].Answer)
		require.Equal(t, data.FinalAnswerStatusSkipped, byHash["txInsufficient"].Status)
		require.Equal(t, data.FinalAnswerStatusSkipped, byHash["txNonRelated"].Status)
	})
}
