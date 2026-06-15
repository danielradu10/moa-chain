package data

type RoundEventType uint8

const (
	UnknownRoundEvent RoundEventType = iota
	StartRoundEvent
	ConsensusMessageEvent
	TimeoutEvent
	StopEvent
)

type RoundEvent struct {
	Type RoundEventType

	RoundKey RoundKey
	Message  ConsensusMessage
	Timeout  *RoundTimeout
}

type RoundTimeout struct {
	RoundKey RoundKey
	Step     Step
}
