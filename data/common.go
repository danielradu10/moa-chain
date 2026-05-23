package data

// MiniRound defines the phases of a round (a mini-round K in a round N)
type MiniRound int

const (
	MiniRoundOne MiniRound = iota
	MiniRoundTwo
	MiniRoundThree
)

// OptionalUint64 holds an optional uint64 value
type OptionalUint64 struct {
	Value    uint64
	HasValue bool
}

// RoundKey defines the key of a specific round, composed by epoch, round, and mini-round.
type RoundKey struct {
	Epoch     uint64
	Round     uint64
	MiniRound uint64
}
