package testscommon

import (
	"errors"
)

type userAccountStateStub struct {
	nonce   uint64
	balance uint64
}

type AccountStateStub struct {
	users map[string]userAccountStateStub
}

func NewAccountStateStub() *AccountStateStub {
	return &AccountStateStub{
		users: make(map[string]userAccountStateStub),
	}
}

func (ass *AccountStateStub) AddAccount(address string, nonce uint64, balance uint64) error {
	ass.users[address] = userAccountStateStub{nonce, balance}
	return nil
}

func (ass *AccountStateStub) UpdateAccount(address string, nonce uint64, balance uint64) error {
	_, ok := ass.users[address]
	if !ok {
		return errors.New("account not exist")
	}

	ass.users[address] = userAccountStateStub{nonce, balance}
	return nil
}

func (ass *AccountStateStub) GetNonceByAddress(address string) (uint64, error) {
	account, ok := ass.users[address]
	if !ok {
		return 0, errors.New("missing nonce")
	}

	return account.nonce, nil
}

func (ass *AccountStateStub) GetBalanceByAddress(address string) (uint64, error) {
	account, ok := ass.users[address]
	if !ok {
		return 0, errors.New("missing balance")
	}

	return account.balance, nil
}
