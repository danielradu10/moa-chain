package validators

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"moa-chain/data"
	"moa-chain/state"
)

type randomnessProvider struct {
	blockchainState state.BlockchainState
}

func NewRandomnessProvider(blockchainState state.BlockchainState) (*randomnessProvider, error) {
	if blockchainState == nil {
		return nil, ErrNilBlockchainState
	}

	return &randomnessProvider{
		blockchainState: blockchainState,
	}, nil
}

func (rp *randomnessProvider) SelectionSeed(roundKey data.RoundKey) ([]byte, error) {
	if rp == nil {
		return nil, ErrNilRandomnessProvider
	}

	currentBlockHeader, err := rp.blockchainState.CurrentBlockHeader()
	if err != nil {
		return nil, err
	}

	if currentBlockHeader == nil {
		return nil, ErrNilCurrentBlockHeader
	}

	if currentBlockHeader.HeaderHash == nil {
		return nil, ErrNilCurrentBlockHash
	}

	hasher := sha256.New()

	hasher.Write([]byte("MOA_CONSENSUS_SELECTION_SEED_V1"))
	hasher.Write(currentBlockHeader.HeaderHash)

	writeUint64(hasher, roundKey.Epoch)
	writeUint64(hasher, roundKey.Round)
	writeUint64(hasher, roundKey.MiniRound)

	return hasher.Sum(nil), nil
}

func writeUint64(hasher hash.Hash, value uint64) {
	var buff [8]byte
	binary.BigEndian.PutUint64(buff[:], value)
	hasher.Write(buff[:])
}

func (rp *randomnessProvider) IsInterfaceNil() bool {
	return rp == nil
}
