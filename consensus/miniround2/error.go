package miniround2

import (
	"errors"
)

// ErrDuplicatedAnswer signals a duplicated answer.
var ErrDuplicatedAnswer = errors.New("duplicated answer")
