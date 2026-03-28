package mempool

import (
	"errors"
)

// ErrNilTransaction signals a nil transaction
var ErrNilTransaction = errors.New("transaction is nil")
