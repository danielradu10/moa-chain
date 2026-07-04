package hashing

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"
	"sort"

	"moa-chain/data"
)

const (
	answerEvidenceHashDomain       = "answer-evidence-v1"
	answerClassificationHashDomain = "answer-classification-v1"
	answerTextHashDomain           = "answer-text-v1"
	protocolPromptHashDomain       = "protocol-prompt-v1"
)

// ComputeAnswerHash computes the answer-text commitment used in a candidate ID.
func ComputeAnswerHash(answer string) []byte {
	hasher := sha256.New()
	writeClassificationString(hasher, answerTextHashDomain)
	writeClassificationString(hasher, answer)

	return hasher.Sum(nil)
}

// ComputeProtocolPromptHash computes a domain-separated prompt commitment.
func ComputeProtocolPromptHash(prompt string) []byte {
	hasher := sha256.New()
	writeClassificationString(hasher, protocolPromptHashDomain)
	writeClassificationString(hasher, prompt)

	return hasher.Sum(nil)
}

// ComputeAnswerEvidenceHash deterministically commits to the complete answer
// evidence certificate. Signers and answer maps are canonicalized before hashing.
func ComputeAnswerEvidenceHash(message *data.AggregatedExecutionResultsMessage) ([]byte, error) {
	if message == nil {
		return nil, ErrInvalidAnswerEvidence
	}
	if len(message.Signers) == 0 ||
		len(message.Signers) != len(message.BlockHashes) ||
		len(message.Signers) != len(message.BlockSignatures) ||
		len(message.Signers) != len(message.Answers) {
		return nil, ErrInvalidAnswerEvidence
	}

	type evidenceEntry struct {
		signer    string
		blockHash []byte
		signature []byte
		answers   data.AnswersTxMessage
	}

	entries := make([]evidenceEntry, 0, len(message.Signers))
	seenSigners := make(map[string]struct{}, len(message.Signers))
	for index, signer := range message.Signers {
		if signer == "" {
			return nil, ErrInvalidAnswerEvidence
		}
		if _, exists := seenSigners[signer]; exists {
			return nil, ErrInvalidAnswerEvidence
		}
		seenSigners[signer] = struct{}{}
		entries = append(entries, evidenceEntry{
			signer:    signer,
			blockHash: message.BlockHashes[index],
			signature: message.BlockSignatures[index],
			answers:   message.Answers[index],
		})
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].signer < entries[right].signer
	})

	hasher := sha256.New()
	writeClassificationString(hasher, answerEvidenceHashDomain)
	writeClassificationUint64(hasher, message.Epoch)
	writeClassificationUint64(hasher, message.Round)
	writeClassificationUint64(hasher, message.MiniRound)
	writeClassificationString(hasher, message.SenderID)
	writeClassificationBytes(hasher, message.CanonicalBlockHash)
	writeClassificationUint64(hasher, uint64(len(entries)))

	for _, entry := range entries {
		writeClassificationString(hasher, entry.signer)
		writeClassificationBytes(hasher, entry.blockHash)
		writeClassificationBytes(hasher, entry.signature)

		txHashes := make([]string, 0, len(entry.answers))
		for txHash := range entry.answers {
			txHashes = append(txHashes, txHash)
		}
		sort.Strings(txHashes)
		writeClassificationUint64(hasher, uint64(len(txHashes)))
		for _, txHash := range txHashes {
			answer := entry.answers[txHash]
			writeClassificationString(hasher, txHash)
			writeClassificationBytes(hasher, answer.TxHash)
			writeClassificationString(hasher, answer.Answer)
			writeClassificationUint64(hasher, answer.ActualConsumption)
		}
	}

	return hasher.Sum(nil), nil
}

// ComputeClassificationVoteHash deterministically commits to a judge vote.
// Assignment input order does not affect the hash. VoteHash and Signature are
// intentionally excluded.
func ComputeClassificationVoteHash(vote *data.AnswerClassificationVote) ([]byte, error) {
	if vote == nil || vote.JudgeID == "" || vote.PromptVersion == "" ||
		len(vote.CanonicalBlockHash) == 0 || len(vote.AnswerEvidenceHash) == 0 || len(vote.PromptHash) == 0 {
		return nil, ErrInvalidClassificationVote
	}
	if len(vote.Assignments) == 0 {
		return nil, ErrInvalidClassificationVote
	}

	assignments := canonicalizeClassificationAssignments(vote.Assignments)
	seenCandidates := make(map[classificationCandidateKey]struct{}, len(assignments))
	for _, assignment := range assignments {
		candidate := assignment.CandidateID
		if candidate.ProducerID == "" || len(candidate.TxHash) == 0 || len(candidate.AnswerHash) != sha256.Size ||
			!assignment.Category.IsValid() {
			return nil, ErrInvalidClassificationVote
		}

		key := classificationCandidateKey{
			producerID: candidate.ProducerID,
			txHash:     string(candidate.TxHash),
			answerHash: string(candidate.AnswerHash),
		}
		if _, exists := seenCandidates[key]; exists {
			return nil, ErrInvalidClassificationVote
		}
		seenCandidates[key] = struct{}{}
	}

	hasher := sha256.New()
	writeClassificationString(hasher, answerClassificationHashDomain)
	writeClassificationUint64(hasher, vote.Epoch)
	writeClassificationUint64(hasher, vote.Round)
	writeClassificationUint64(hasher, vote.MiniRound)
	writeClassificationBytes(hasher, vote.CanonicalBlockHash)
	writeClassificationBytes(hasher, vote.AnswerEvidenceHash)
	writeClassificationString(hasher, vote.JudgeID)
	writeClassificationString(hasher, vote.PromptVersion)
	writeClassificationBytes(hasher, vote.PromptHash)
	writeClassificationString(hasher, vote.ModelMetadata)
	writeClassificationUint64(hasher, uint64(len(assignments)))
	for _, assignment := range assignments {
		writeCandidateID(hasher, assignment.CandidateID)
		writeClassificationString(hasher, string(assignment.Category))
	}

	return hasher.Sum(nil), nil
}

type classificationCandidateKey struct {
	producerID string
	txHash     string
	answerHash string
}

func canonicalizeClassificationAssignments(
	assignments []data.AnswerClassificationAssignment,
) []data.AnswerClassificationAssignment {
	ordered := append([]data.AnswerClassificationAssignment(nil), assignments...)
	sort.Slice(ordered, func(left, right int) bool {
		return data.CompareAnswerCandidateIDs(ordered[left].CandidateID, ordered[right].CandidateID) < 0
	})

	return ordered
}

func writeCandidateID(hasher hash.Hash, candidate data.AnswerCandidateID) {
	writeClassificationString(hasher, candidate.ProducerID)
	writeClassificationBytes(hasher, candidate.TxHash)
	writeClassificationBytes(hasher, candidate.AnswerHash)
}

func writeClassificationUint64(hasher hash.Hash, value uint64) {
	var buffer [8]byte
	binary.BigEndian.PutUint64(buffer[:], value)
	_, _ = hasher.Write(buffer[:])
}

func writeClassificationBytes(hasher hash.Hash, value []byte) {
	writeClassificationUint64(hasher, uint64(len(value)))
	_, _ = hasher.Write(value)
}

func writeClassificationString(hasher hash.Hash, value string) {
	writeClassificationBytes(hasher, []byte(value))
}
