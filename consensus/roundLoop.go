package consensus

import (
	"log/slog"

	"moa-chain/data"
	"moa-chain/logging"
)

type RoundLoop struct {
	handler *roundHandler
	inbox   chan data.RoundEvent
	errCh   chan error
	logger  *slog.Logger
}

func NewRoundLoop(handler *roundHandler, inbox chan data.RoundEvent, loggers ...*slog.Logger) *RoundLoop {
	return &RoundLoop{
		handler: handler,
		inbox:   inbox,
		errCh:   make(chan error, 32),
		logger:  logging.FromOptional(loggers...),
	}
}

func (rl *RoundLoop) Errors() <-chan error {
	return rl.errCh
}

// Shutdown stops the event loop and waits for all in-flight work to finish.
// It sends StopEvent to the inbox, waits for Run to return (via the provided
// done channel), then calls WaitForPendingWork on the MR2 handler to join any
// background judge goroutines.
func (rl *RoundLoop) Shutdown(done <-chan struct{}) {
	rl.inbox <- data.RoundEvent{Type: data.StopEvent}
	<-done
	rl.handler.miniRoundTwoHandler.WaitForPendingWork()
}

// WaitForPendingWork blocks until all in-flight MR2 judge goroutines have
// finished. Use this when the event loop has already stopped (e.g. it
// received StopEvent from another path).
func (rl *RoundLoop) WaitForPendingWork() {
	rl.handler.miniRoundTwoHandler.WaitForPendingWork()
}

// LiveState returns the current round key and consensus step as an atomic
// snapshot. Safe to call from any goroutine while the loop is running.
func (rl *RoundLoop) LiveState() (data.RoundKey, data.Step) {
	return rl.handler.liveState()
}

func (rl *RoundLoop) Run() {
	rl.logger.Info("consensus.RoundLoop.Run started")
	defer rl.logger.Info("consensus.RoundLoop.Run stopped")

	for event := range rl.inbox {
		var err error
		rl.logger.Debug("consensus.RoundLoop.Run received event", "eventType", event.Type, "roundKey", event.RoundKey)

		switch event.Type {
		case data.StartRoundEvent:
			err = rl.handler.StartRound(event.RoundKey)

		case data.ConsensusMessageEvent:
			err = rl.handler.HandleMessage(event.Message)

		case data.TimeoutEvent:
			err = rl.handler.OnTimeout(event.Timeout.RoundKey, event.Timeout.Step)

		case data.ClassificationGracePeriodElapsedEvent:
			err = rl.handler.HandleClassificationGracePeriodElapsed(event.RoundKey)

		case data.StopEvent:
			rl.logger.Info("consensus.RoundLoop.Run received stop event")
			return

		default:
			rl.logger.Error("consensus.RoundLoop.Run received unknown event", "eventType", event.Type)
			panic("unhandled default case")
		}

		if err != nil {
			rl.logger.Error("consensus.RoundLoop.Run handler returned error", "eventType", event.Type, "error", err)
			rl.errCh <- err
		}
	}
}
