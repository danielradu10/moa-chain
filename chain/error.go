package chain

import "errors"

// ErrNilBlock signals that a nil block was provided to Append.
var ErrNilBlock = errors.New("nil block")

// ErrEmptyChain signals that Head was called on a chain with no blocks.
var ErrEmptyChain = errors.New("chain is empty")

// ErrNonContiguousBlock signals that the appended block's round number is not
// exactly head.Round + 1.
var ErrNonContiguousBlock = errors.New("non-contiguous block")

// ErrPreviousHashMismatch signals that the appended block's PreviousHash does
// not match the current head's HeaderHash.
var ErrPreviousHashMismatch = errors.New("previous hash mismatch")
