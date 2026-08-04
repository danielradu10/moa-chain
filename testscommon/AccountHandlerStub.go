package testscommon

import (
	"errors"
)

type AccountHandlerStub struct {
	nonce   uint64
	balance uint64
}

func NewAccountHandlerStub(nonce uint64, balance uint64) *AccountHandlerStub {
	return &AccountHandlerStub{
		nonce:   nonce,
		balance: balance,
	}
}

func (ta *AccountHandlerStub) Nonce() (uint64, error) {
	return ta.nonce, nil
}

func (ta *AccountHandlerStub) Balance() (uint64, error) {
	return ta.balance, nil
}

func (ta *AccountHandlerStub) IncreaseNonce(increment uint64) error {
	ta.nonce += increment
	return nil
}

func (ta *AccountHandlerStub) IncreaseBalance(increment uint64) error {
	ta.balance += increment
	return nil
}

func (ta *AccountHandlerStub) DecreaseBalance(decrement uint64) error {
	if decrement > ta.balance {
		return errors.New("insufficient balance")
	}

	ta.balance -= decrement
	return nil
}
