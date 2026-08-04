package testscommon

import (
	"errors"

	"moa-chain/state"
)

type AccountsSnapshotStub struct {
	Accounts       map[string]*AccountHandlerStub
	EscrowAccount  *AccountHandlerStub
	LoadAccountErr error
	LoadEscrowErr  error
	CommitErr      error

	CommitCalled  bool
	DiscardCalled bool
}

func (ass *AccountsSnapshotStub) LoadAccount(address string) (state.AccountHandler, error) {
	if ass.LoadAccountErr != nil {
		return nil, ass.LoadAccountErr
	}

	account, ok := ass.Accounts[address]
	if !ok {
		return nil, errors.New("account not found")
	}

	return account, nil
}

func (ass *AccountsSnapshotStub) LoadEscrowAccount() (state.AccountHandler, error) {
	if ass.LoadEscrowErr != nil {
		return nil, ass.LoadEscrowErr
	}

	if ass.EscrowAccount == nil {
		return nil, errors.New("escrow account not found")
	}

	return ass.EscrowAccount, nil
}

func (ass *AccountsSnapshotStub) Commit() error {
	ass.CommitCalled = true
	return ass.CommitErr
}

func (ass *AccountsSnapshotStub) Discard() {
	ass.DiscardCalled = true
}
