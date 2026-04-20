package testscommon

import "moa-chain/state"

type AccountsSnapshotFactoryStub struct {
	Snapshot          state.AccountsSnapshot
	CreateSnapshotErr error
	CreateCalled      bool
}

func (asfs *AccountsSnapshotFactoryStub) CreateSnapshot() (state.AccountsSnapshot, error) {
	asfs.CreateCalled = true

	if asfs.CreateSnapshotErr != nil {
		return nil, asfs.CreateSnapshotErr
	}

	return asfs.Snapshot, nil
}
