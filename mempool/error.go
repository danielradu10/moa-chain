package mempool

import (
	"errors"
)

// ErrNilTransaction signals a nil transaction
var ErrNilTransaction = errors.New("transaction is nil")

// ErrNilSender signals a nil sender
var ErrNilSender = errors.New("sender is nil")

// ErrSenderDoesNotExist signals an inexistent sender
var ErrSenderDoesNotExist = errors.New("sender does not exist")

// ErrNilTxComparator signals a nil transaction comparator
var ErrNilTxComparator = errors.New("tx comparator is nil")
