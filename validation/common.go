package validation

// MiniRound defines the phases of a round (a mini-round K in a round N)
type MiniRound int

const (
	MiniRoundOne MiniRound = iota
	MiniRoundTwo
	MiniRoundThree
)
