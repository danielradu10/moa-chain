package testscommon

import "moa-chain/data"

type BlockchainStateStub struct {
	CurrentBlockHeaderValue *data.ChainBlockHeader
	CurrentBlockHeaderErr   error

	CurrentBlockValue *data.Block
	CurrentBlockErr   error

	CurrentRoundValue     uint64
	CurrentMiniRoundValue uint64
	CurrentEpochValue     uint64
}

func (bss *BlockchainStateStub) CurrentBlockHeader() (*data.ChainBlockHeader, error) {
	if bss.CurrentBlockHeaderErr != nil {
		return nil, bss.CurrentBlockHeaderErr
	}

	return bss.CurrentBlockHeaderValue, nil
}

func (bss *BlockchainStateStub) CurrentRound() (uint64, error) {
	return bss.CurrentRoundValue, nil
}

func (bss *BlockchainStateStub) CurrentMiniRound() (uint64, error) {
	return bss.CurrentMiniRoundValue, nil
}

func (bss *BlockchainStateStub) CurrentEpoch() (uint64, error) {
	return bss.CurrentEpochValue, nil
}
