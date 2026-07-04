package miniround2

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"hash"
	"sort"

	"moa-chain/data"
)

const (
	answerEvidenceHashDomain       = "answer-evidence-v1"
	answerClassificationHashDomain = "answer-classification-v1"
	answerTextHashDomain           = "answer-text-v1"
)

// ComputeAnswerHash computes the answer-text commitment used in a candidate ID.
func ComputeAnswerHash(answer string) []byte {
	hasher := sha256.New()
	writeClassificationString(hasher, answerTextHashDomain)
	writeClassificationString(hasher, answer)

	return hasher.Sum(nil)
}

// ComputeAnswerEvidenceHash deterministically commits to the complete answer
// evidence certificate. Signers and answer maps are canonicalized before hashing.
func ComputeAnswerEvidenceHash(message *data.AggregatedExecutionResultsMessage) ([]byte, error) {
	if message == nil {
		return nil, fmt.Errorf("%w: nil message", ErrInvalidAnswerEvidence)
	}
	if len(message.Signers) == 0 ||
		len(message.Signers) != len(message.BlockHashes) ||
		len(message.Signers) != len(message.BlockSignatures) ||
		len(message.Signers) != len(message.Answers) {
		return nil, fmt.Errorf("%w: misaligned certificate", ErrInvalidAnswerEvidence)
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
			return nil, fmt.Errorf("%w: empty signer", ErrInvalidAnswerEvidence)
		}
		if _, exists := seenSigners[signer]; exists {
			return nil, fmt.Errorf("%w: duplicated signer %q", ErrInvalidAnswerEvidence, signer)
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
		return nil, ErrMissingAnswerCandidate
	}

	assignments := CanonicalizeClassificationAssignments(vote.Assignments)
	expectedCandidates := make([]data.AnswerCandidateID, 0, len(assignments))
	for _, assignment := range assignments {
		expectedCandidates = append(expectedCandidates, assignment.CandidateID)
	}
	if err := ValidateClassificationAssignments(expectedCandidates, assignments); err != nil {
		return nil, err
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
