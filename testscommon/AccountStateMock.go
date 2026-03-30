package testscommon

type AccountStateMock struct {
	GetNonceByAddressCalled   func(string) (uint64, error)
	GetBalanceByAddressCalled func(string) (uint64, error)
}

func (asm *AccountStateMock) GetNonceByAddress(address string) (uint64, error) {
	if asm.GetNonceByAddressCalled != nil {
		return asm.GetNonceByAddressCalled(address)
	}

	return 0, nil
}

func (asm *AccountStateMock) GetBalanceByAddress(address string) (uint64, error) {
	if asm.GetBalanceByAddressCalled != nil {
		return asm.GetBalanceByAddressCalled(address)
	}

	return 0, nil
}
