package hashing

import "errors"

var (
	// ErrNilBlock signals that a nil block-related structure was provided for hashing.
	ErrNilBlock = errors.New("nil block")

	// ErrNilTransaction signals that a nil transaction was provided for hashing.
	ErrNilTransaction = errors.New("nil transaction")

	// ErrInvalidAnswerEvidence signals malformed answer evidence hash input.
	ErrInvalidAnswerEvidence = errors.New("invalid answer evidence")

	// ErrInvalidClassificationVote signals malformed classification vote hash input.
	ErrInvalidClassificationVote = errors.New("invalid classification vote")
)
