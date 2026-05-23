package broadcast

import (
	"errors"
)

// ErrInvalidValidator signals an invalid validator.
var ErrInvalidValidator = errors.New("invalid validator")

// ErrValidatorAlreadyExists signals a validator that already exists.
var ErrValidatorAlreadyExists = errors.New("validator already exists")

// ErrNilConsensusMessage signals a nil consensus message
var ErrNilConsensusMessage = errors.New("consensus message is nil")

// ErrNilChannel signals a nil channel
var ErrNilChannel = errors.New("channel is nil")
