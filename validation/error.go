package validation

import (
	"errors"
)

// ErrWrongTransactionNonce signals a wrong transaction nonce
var ErrWrongTransactionNonce = errors.New("wrong transaction nonce")

// ErrWrongTransactionBalance signals a wrong transaction balance
var ErrWrongTransactionBalance = errors.New("wrong transaction balance")

// ErrNilBlock signals a nil block
var ErrNilBlock = errors.New("nil block")

// ErrBlockNonceNotContinuous signals a discontinuous block nonce
var ErrBlockNonceNotContinuous = errors.New("block nonce not contiguous")

// ErrWrongBlockRound signals a discontinuous block round
var ErrWrongBlockRound = errors.New("discontinuous block round")

// ErrWrongMiniBlockRound signals a discontinuous mini block round
var ErrWrongMiniBlockRound = errors.New("discontinuous mini block round")

// ErrDiscontinuousHash signals a discontinuous hash between blocks
var ErrDiscontinuousHash = errors.New("discontinuous hash")

// ErrDiscontinuousRootHash signals a discontinuous root hash between blocks
var ErrDiscontinuousRootHash = errors.New("discontinuous root hash")
