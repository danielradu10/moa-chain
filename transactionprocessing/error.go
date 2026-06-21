package transactionprocessing

import (
	"errors"
)

// ErrNotImplemented signals an unimplemented method / case
var ErrNotImplemented = errors.New("not implemented")

// ErrUnsupportedMiniRound signals an unsupported mini round
var ErrUnsupportedMiniRound = errors.New("unsupported mini round")

// ErrWrongTransactionNonce signals a wrong transaction nonce
var ErrWrongTransactionNonce = errors.New("wrong transaction nonce")

// ErrWrongTransactionBalance signals a wrong transaction balance
var ErrWrongTransactionBalance = errors.New("wrong transaction balance")

// ErrLeaderGeneratedTooManyLabels signals that a transactions has too many labels
var ErrLeaderGeneratedTooManyLabels = errors.New("leader generated too many labels")

// ErrValidatorGeneratedTooManyLabels signals that a transactions has too many labels
var ErrValidatorGeneratedTooManyLabels = errors.New("validator generated too many labels")

// ErrLeaderProposedDuplicatedLabels signals that the leader proposed duplicated labels on same tx
var ErrLeaderProposedDuplicatedLabels = errors.New("leader generated duplicated labels")

// ErrValidatorGeneratedDuplicatedLabels signals that the validator generated duplicated labels on same tx
var ErrValidatorGeneratedDuplicatedLabels = errors.New("validator generated duplicated labels")

// ErrUnknownLabel signals that a transaction has an incorrect label
var ErrUnknownLabel = errors.New("unknown label")

// ErrLabelIsNotValid signals that a label is invalid
var ErrLabelIsNotValid = errors.New("label is not valid")

// ErrTxsDoNotRespectProtocolOrder signals that the proposed tx do not respect protcol order
var ErrTxsDoNotRespectProtocolOrder = errors.New("txs do not respect protocol order")

// ErrNilAgent signals a nil agent.
var ErrNilAgent = errors.New("nil agent")
