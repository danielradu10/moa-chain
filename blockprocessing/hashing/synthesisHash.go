package hashing

import (
	"crypto/sha256"

	"moa-chain/data"
)

const (
	synthesisBlockHashDomain = "synthesis-block-v1"
	synthesisVoteHashDomain  = "synthesis-vote-v1"
)

// ComputeSynthesisBlockHash deterministically commits to the leader's proposed
// synthesis payload. SynthesisBlockHash and Signature are excluded; all other
// fields are included. SynthesizedAnswers are expected to be pre-sorted by
// transaction hash by the caller.
func ComputeSynthesisBlockHash(msg *data.ProposedSynthesisMessage) ([]byte, error) {
	if msg == nil {
		return nil, ErrNilBlock
	}

	hasher := sha256.New()
	writeClassificationString(hasher, synthesisBlockHashDomain)
	writeClassificationUint64(hasher, msg.Epoch)
	writeClassificationUint64(hasher, msg.Round)
	writeClassificationUint64(hasher, msg.MiniRound)
	writeClassificationString(hasher, msg.SenderID)
	writeClassificationBytes(hasher, msg.CanonicalBlockHash)
	writeClassificationBytes(hasher, msg.AnswerEvidenceHash)

	writeClassificationUint64(hasher, uint64(len(msg.SynthesizedAnswers)))
	for _, sa := range msg.SynthesizedAnswers {
		writeClassificationBytes(hasher, sa.TxHash)
		writeClassificationString(hasher, sa.Answer)
	}

	return hasher.Sum(nil), nil
}

// ComputeSynthesisVoteHash deterministically commits to a validator's binary
// synthesis approval. VoteHash and Signature are excluded.
func ComputeSynthesisVoteHash(vote *data.SynthesisVote) ([]byte, error) {
	if vote == nil {
		return nil, ErrNilBlock
	}

	hasher := sha256.New()
	writeClassificationString(hasher, synthesisVoteHashDomain)
	writeClassificationUint64(hasher, vote.Epoch)
	writeClassificationUint64(hasher, vote.Round)
	writeClassificationUint64(hasher, vote.MiniRound)
	writeClassificationString(hasher, vote.VoterID)
	writeClassificationBytes(hasher, vote.SynthesisBlockHash)

	return hasher.Sum(nil), nil
}
