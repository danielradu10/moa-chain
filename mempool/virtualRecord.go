package mempool

import (
	"moa-chain/common"
)

type virtualRecord struct {
	initialNonce   common.OptionalUint64
	initialBalance uint64

	currentNonce       common.OptionalUint64
	accumulatedBalance uint64
}

func newVirtualRecord(
	initialNonce uint64,
	initialBalance uint64,
	currentNonce uint64,
) *virtualRecord {
	return &virtualRecord{
		initialNonce: common.OptionalUint64{
			Value:    initialNonce,
			HasValue: true,
		},
		currentNonce: common.OptionalUint64{
			Value:    currentNonce,
			HasValue: true,
		},
		initialBalance:     initialBalance,
		accumulatedBalance: 0,
	}
}

func (vr *virtualRecord) accumulateBalance(amount uint64) {
	vr.accumulatedBalance += amount
}

func (vr *virtualRecord) updateNonce(lastNonce uint64) {
	vr.currentNonce.Value = lastNonce
}
