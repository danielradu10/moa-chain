package consensus

import (
	"moa-chain/data"
)

type RoundLoop struct {
	handler *roundHandler
	inbox   chan data.RoundEvent
	errCh   chan error
}

func NewRoundLoop(handler *roundHandler, inbox chan data.RoundEvent) *RoundLoop {
	return &RoundLoop{
		handler: handler,
		inbox:   inbox,
		errCh:   make(chan error, 32),
	}
}

func (rl *RoundLoop) Errors() <-chan error {
	return rl.errCh
}

func (rl *RoundLoop) Run() {
	for event := range rl.inbox {
		var err error

		switch event.Type {
		case data.StartRoundEvent:
			err = rl.handler.StartRound(event.RoundKey)

		case data.ConsensusMessageEvent:
			err = rl.handler.HandleMessage(event.Message)

		case data.TimeoutEvent:
			err = rl.handler.OnTimeout(event.Timeout.RoundKey, event.Timeout.Step)

		case data.StopEvent:
			return

		default:
			panic("unhandled default case")
		}

		if err != nil {
			rl.errCh <- err
		}
	}
}
