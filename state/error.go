package state

import (
	"errors"
)

// ErrNoProposedBlockForRoundKey signals that no Block was cached for a specific RoundKey.
var ErrNoProposedBlockForRoundKey = errors.New("no proposed block")

// ErrBlockAlreadyExistsForRoundKey signals that there already exists a block for the RoundKey.
var ErrBlockAlreadyExistsForRoundKey = errors.New("block already exists for round")

// ErrVoteAlreadyExistsForSigner signals that a specific vote already exists.
var ErrVoteAlreadyExistsForSigner = errors.New("vote already exists for signer")

// ErrNoVotesForCurrentRoundKey signals that there are no votes for the current round.
var ErrNoVotesForCurrentRoundKey = errors.New("no votes for current round")

// ErrNilBlock signals a nil block.
var ErrNilBlock = errors.New("nil block")

// ErrNilVote signals a nil block.
var ErrNilVote = errors.New("nil vote")

// ErrNilExecutedPromptsMessage signals a nil executed prompts message.
var ErrNilExecutedPromptsMessage = errors.New("nil executed prompts message")

// ErrExecutedPromptsAlreadyExistsForSender signals duplicated executed prompts from the same sender.
var ErrExecutedPromptsAlreadyExistsForSender = errors.New("executed prompts already exists for sender")

// ErrNoExecutedPromptsForCurrentRoundKey signals that there are no executed prompts for the current round.
var ErrNoExecutedPromptsForCurrentRoundKey = errors.New("no executed prompts for current round")

// ErrNilAnswerEvidence signals missing aggregated answer evidence.
var ErrNilAnswerEvidence = errors.New("nil answer evidence")

// ErrAnswerEvidenceAlreadyExists signals duplicate answer evidence for one round.
var ErrAnswerEvidenceAlreadyExists = errors.New("answer evidence already exists for round")

// ErrNoAnswerEvidenceForCurrentRoundKey signals that classification started without verified evidence.
var ErrNoAnswerEvidenceForCurrentRoundKey = errors.New("no answer evidence for current round")

// ErrNilAnswerClassificationVote signals a nil classification vote.
var ErrNilAnswerClassificationVote = errors.New("nil answer classification vote")

// ErrAnswerClassificationVoteAlreadyExistsForJudge signals a duplicate judge vote.
var ErrAnswerClassificationVoteAlreadyExistsForJudge = errors.New("answer classification vote already exists for judge")

// ErrNoAnswerClassificationVotesForCurrentRoundKey signals missing classification votes.
var ErrNoAnswerClassificationVotesForCurrentRoundKey = errors.New("no answer classification votes for current round")

// ErrNilAnswerClassificationCertificate signals a nil classification certificate.
var ErrNilAnswerClassificationCertificate = errors.New("nil answer classification certificate")

// ErrAnswerClassificationCertificateAlreadyExists signals a duplicate classification certificate.
var ErrAnswerClassificationCertificateAlreadyExists = errors.New("answer classification certificate already exists")
