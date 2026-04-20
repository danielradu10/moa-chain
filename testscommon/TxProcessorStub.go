package testscommon

import (
	"moa-chain/data"
	"moa-chain/validation"
)

type TxProcessorStub struct {
	ProcessTransactionHandler func(tx data.Transaction, miniRound validation.MiniRound) (uint64, error)
	Called                    bool
}

func (tps *TxProcessorStub) ProcessTransaction(tx data.Transaction, miniRound validation.MiniRound) (uint64, error) {
	tps.Called = true

	if tps.ProcessTransactionHandler != nil {
		return tps.ProcessTransactionHandler(tx, miniRound)
	}

	return 0, nil
}
