package testscommon

import (
	"errors"

	"moa-chain/state"
)

type AccountsProviderStub struct {
	Accounts       map[string]*AccountHandlerStub
	EscrowAccount  *AccountHandlerStub
	LoadAccountErr error
	LoadEscrowErr  error
}

func (tap *AccountsProviderStub) LoadAccount(address string) (state.AccountHandler, error) {
	if tap.LoadAccountErr != nil {
		return nil, tap.LoadAccountErr
	}

	account, ok := tap.Accounts[address]
	if !ok {
		return nil, errors.New("account not found")
	}

	return account, nil
}

func (tap *AccountsProviderStub) LoadEscrowAccount() (state.AccountHandler, error) {
	if tap.LoadEscrowErr != nil {
		return nil, tap.LoadEscrowErr
	}

	return tap.EscrowAccount, nil
}
