package mempool

import (
	"moa-chain/data"
)

type virtualRecord struct {
	initialNonce   data.OptionalUint64
	initialBalance uint64

	currentNonce       data.OptionalUint64
	accumulatedBalance uint64
}

func newVirtualRecord(
	initialNonce uint64,
	initialBalance uint64,
	currentNonce uint64,
) *virtualRecord {
	return &virtualRecord{
		initialNonce: data.OptionalUint64{
			Value:    initialNonce,
			HasValue: true,
		},
		currentNonce: data.OptionalUint64{
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
