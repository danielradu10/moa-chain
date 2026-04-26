package blockprocessing

import (
	"errors"
)

// ErrNilBlock signals a nil block
var ErrNilBlock = errors.New("nil block")

// ErrBlockNonceNotContinuous signals a discontinuous block nonce
var ErrBlockNonceNotContinuous = errors.New("block nonce not contiguous")

// ErrWrongMiniBlockRound signals a discontinuous mini block round
var ErrWrongMiniBlockRound = errors.New("discontinuous mini block round")

// ErrDiscontinuousHash signals a discontinuous hash between blocks
var ErrDiscontinuousHash = errors.New("discontinuous hash")

// ErrDiscontinuousRootHash signals a discontinuous root hash between blocks
var ErrDiscontinuousRootHash = errors.New("discontinuous root hash")

// ErrBlockConsumptionReached signals that the block consumption was reached
var ErrBlockConsumptionReached = errors.New("block consumption reached")

// ErrInvalidNumSubdomains signals invalid subdomains
var ErrInvalidNumSubdomains = errors.New("invalid number of subdomains")

// ErrInvalidSubdomain signals invalid subdomain
var ErrInvalidSubdomain = errors.New("invalid subdomain")

// ErrInvalidFrequencyOfSubdomain signals an invalid frequency of the subdomain
var ErrInvalidFrequencyOfSubdomain = errors.New("invalid frequency of subdomain")

// ErrDuplicatedTransaction signals a duplicated transaction in the proposed block
var ErrDuplicatedTransaction = errors.New("duplicated transaction")

// ErrNilTransaction signals a nil transaction
var ErrNilTransaction = errors.New("nil transaction")
