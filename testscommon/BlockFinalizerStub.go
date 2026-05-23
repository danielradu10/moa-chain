package testscommon

import (
	"sync"

	"moa-chain/data"
)

type BlockFinalizerStub struct {
	mutex sync.Mutex

	FinalizedBlock *data.Block
	FinalizeCalled bool
}

func (stub *BlockFinalizerStub) FinalizeBlock(block *data.Block) error {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	stub.FinalizeCalled = true
	stub.FinalizedBlock = block

	return nil
}

func (stub *BlockFinalizerStub) GetFinalizedBlock() *data.Block {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	return stub.FinalizedBlock
}

func (stub *BlockFinalizerStub) WasFinalizeCalled() bool {
	stub.mutex.Lock()
	defer stub.mutex.Unlock()

	return stub.FinalizeCalled
}
