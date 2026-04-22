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
