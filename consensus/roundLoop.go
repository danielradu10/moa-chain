package consensus

import (
	"moa-chain/data"
)

type RoundLoop struct {
	handler roundHandler
	inbox   chan data.RoundEvent
}

func NewRoundLoop(handler roundHandler) *RoundLoop {
	return &RoundLoop{
		handler: handler,
		inbox:   make(chan data.RoundEvent, 1024),
	}
}

func (rl *RoundLoop) Inbox() chan<- data.RoundEvent {
	return rl.inbox
}

func (rl *RoundLoop) Run() {
	for event := range rl.inbox {
		switch event.Type {
		case data.StartRoundEvent:
			_ = rl.handler.StartRound(event.RoundKey)

		case data.ConsensusMessageEvent:
			_ = rl.handler.HandleMessage(event.Message)

		case data.TimeoutEvent:
			_ = rl.handler.OnTimeout(event.Timeout.RoundKey, event.Timeout.Step)

		case data.StopEvent:
			return
		default:
			panic("unhandled default case")
		}
	}
}
