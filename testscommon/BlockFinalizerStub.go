package testscommon

import (
	"sync"

	"moa-chain/data"
)

type BlockFinalizerStub struct {
	mutex sync.Mutex

	FinalizedBlock *data.BlockOnChain
	FinalizeCalled bool
}

func (stub *BlockFinalizerStub) FinalizeBlockMROne(block *data.BlockOnChain) error {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	stub.FinalizeCalled = true
	stub.FinalizedBlock = block

	return nil
}

func (stub *BlockFinalizerStub) GetFinalizedBlock() *data.BlockOnChain {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	return stub.FinalizedBlock
}

func (stub *BlockFinalizerStub) WasFinalizeCalled() bool {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	return stub.FinalizeCalled
}
