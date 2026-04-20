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

// ErrWrongMiniBlockRound signals a discontinuous mini block round
var ErrWrongMiniBlockRound = errors.New("discontinuous mini block round")

// ErrDiscontinuousHash signals a discontinuous hash between blocks
var ErrDiscontinuousHash = errors.New("discontinuous hash")

// ErrDiscontinuousRootHash signals a discontinuous root hash between blocks
var ErrDiscontinuousRootHash = errors.New("discontinuous root hash")

// ErrUnsupportedMiniRound signals an unsupported mini round
var ErrUnsupportedMiniRound = errors.New("unsupported mini round")

// ErrNotImplemented signals an unimplemented method / case
var ErrNotImplemented = errors.New("not implemented")

// ErrBlockConsumptionReached signals that the block consumption was reached
var ErrBlockConsumptionReached = errors.New("block consumption reached")

// ErrLeaderGeneratedTooManyLabels signals that a transactions has too many labels
var ErrLeaderGeneratedTooManyLabels = errors.New("leader generated too many labels")

// ErrValidatorGeneratedTooManyLabels signals that a transactions has too many labels
var ErrValidatorGeneratedTooManyLabels = errors.New("validator generated too many labels")

// ErrUnknownLabel signals that a transaction has an incorrect label
var ErrUnknownLabel = errors.New("unknown label")

// ErrLabelIsNotValid signals that a label is invalid
var ErrLabelIsNotValid = errors.New("label is not valid")

// ErrInvalidNumSubdomains signals invalid subdomains
var ErrInvalidNumSubdomains = errors.New("invalid number of subdomains")

// ErrInvalidSubdomain signals invalid subdomain
var ErrInvalidSubdomain = errors.New("invalid subdomain")

// ErrInvalidFrequencyOfSubdomain signals an invalid frequency of the subdomain
var ErrInvalidFrequencyOfSubdomain = errors.New("invalid frequency of subdomain")

// ErrDuplicatedTransaction signals a duplicated transaction in the proposed block
var ErrDuplicatedTransaction = errors.New("duplicated transaction")

// ErrTxsDoNotRespectProtocolOrder signals that the proposed tx do not respect protcol order
var ErrTxsDoNotRespectProtocolOrder = errors.New("txs do not respect protocol order")

// ErrLeaderProposedDuplicatedLabels signals that the leader proposed duplicated labels on same tx
var ErrLeaderProposedDuplicatedLabels = errors.New("leader generated duplicated labels")

// ErrValidatorGeneratedDuplicatedLabels signals that the validator generated duplicated labels on same tx
var ErrValidatorGeneratedDuplicatedLabels = errors.New("validator generated duplicated labels")
