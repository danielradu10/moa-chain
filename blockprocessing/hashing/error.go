package hashing

import "errors"

var (
	// ErrNilBlock signals that a nil block-related structure was provided for hashing.
	ErrNilBlock = errors.New("nil block")

	// ErrNilTransaction signals that a nil transaction was provided for hashing.
	ErrNilTransaction = errors.New("nil transaction")
)
