package broadcast

import (
	"moa-chain/data"
)

type peerRegistry struct {
	registry map[string]chan<- data.RoundEvent
}

func NewPeerRegistry() *peerRegistry {
	return &peerRegistry{
		registry: make(map[string]chan<- data.RoundEvent),
	}
}

// GetChannel returns the communication channel of a validator.
func (pr *peerRegistry) GetChannel(validatorID string) (chan<- data.RoundEvent, error) {
	_, ok := pr.registry[validatorID]
	if !ok {
		return nil, ErrInvalidValidator
	}

	return pr.registry[validatorID], nil
}

func (pr *peerRegistry) Register(validatorID string, channel chan<- data.RoundEvent) error {
	_, ok := pr.registry[validatorID]
	if ok {
		return ErrValidatorAlreadyExists
	}

	pr.registry[validatorID] = channel
	return nil
}

func (pr *peerRegistry) Unregister(validatorID string) {
	delete(pr.registry, validatorID)
}
