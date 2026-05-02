package validators

import (
	"errors"
)

// ErrLeaderNotSet signals that the leader is not set
var ErrLeaderNotSet = errors.New("leader not set")

// ErrInvalidValidator signals an invalid validator.
var ErrInvalidValidator = errors.New("invalid validator")
