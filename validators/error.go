package validators

import (
	"errors"
)

// ErrLeaderNotSet signals that the leader is not set
var ErrLeaderNotSet = errors.New("leader not set")

// ErrInvalidValidator signals an invalid validator.
var ErrInvalidValidator = errors.New("invalid validator")

// ErrLeaderNotSelected signals that the leader was not selected yet.
var ErrLeaderNotSelected = errors.New("leader not selected")

// ErrConsensusGroupNotSelected signals that the consensus group was not selected.
var ErrConsensusGroupNotSelected = errors.New("consensus group not selected")

// ErrNilBlockchainState signals a nil blockchain state.
var ErrNilBlockchainState = errors.New("nil blockchain state")

// ErrNilRandomnessProvider signals a nil randomness provider.
var ErrNilRandomnessProvider = errors.New("nil randomness provider")

// ErrNilCurrentBlockHeader signals a nil current block header.
var ErrNilCurrentBlockHeader = errors.New("nil current block header")

// ErrNilCurrentBlockHash signals a nil current block hash.
var ErrNilCurrentBlockHash = errors.New("nil current block hash")

// ErrNilSelectionSeed signals a nil selection seed.
var ErrNilSelectionSeed = errors.New("nil selection seed")

// ErrEmptyExpandedList signals an empty expanded list.
var ErrEmptyExpandedList = errors.New("empty expanded list")

// ErrInvalidConsensusGroupSize signals an invalid group size.
var ErrInvalidConsensusGroupSize = errors.New("invalid consensus group size")

// ErrNotEnoughValidators signals that there are not enough validators for a selection.
var ErrNotEnoughValidators = errors.New("not enough validators")

// ErrNotEnoughValidatorsSelected signals not enough selected validators.
var ErrNotEnoughValidatorsSelected = errors.New("not enough validators selected")

// ErrTotalFreqIsZero signals a zero sum of frequencies.
var ErrTotalFreqIsZero = errors.New("total frequency is zero")
