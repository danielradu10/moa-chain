package proposing

import (
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/transactionprocessing/processor"
)

type blockCreator struct {
	blockprocessing.Base
	mempool mempool.Mempool
}

// NewBlockCreator creates a new block creator
func NewBlockCreator(
	base blockprocessing.Base,
	mempool mempool.Mempool,
) *blockCreator {
	return &blockCreator{
		Base:    base,
		mempool: mempool,
	}
}

// ProposeBlock proposes a block
func (bc *blockCreator) ProposeBlock() (*data.Block, error) {
	proposedHeader, err := bc.createProposedHeader()
	if err != nil {
		return nil, err
	}

	proposedBody, err := bc.createProposedBody()
	if err != nil {
		return nil, err
	}

	// TODO extract new root hash!! analyze how the account state should behave

	headerHash, err := bc.hashProposedBlock(proposedBody, proposedHeader)
	if err != nil {
		return nil, err
	}
	proposedHeader.HeaderHash = headerHash

	// return proposed block
	proposedBlock := &data.Block{
		Header: *proposedHeader,
		Body:   *proposedBody,
	}

	return proposedBlock, nil
}

func (bc *blockCreator) createProposedHeader() (*data.BlockHeader, error) {
	// set nonce, round, mini round, epoch, previous header hash, previous root hash
	currentBlock, err := bc.BlockchainState.CurrentBlockHeader()
	if err != nil {
		return nil, err
	}

	currentNonce := currentBlock.Nonce
	currentHash := currentBlock.HeaderHash
	currentRootHash := currentBlock.RootHash

	currentEpoch, err := bc.BlockchainState.CurrentEpoch()
	if err != nil {
		return nil, err
	}

	currentRound, err := bc.BlockchainState.CurrentRound()
	if err != nil {
		return nil, err
	}

	currentMiniRound, err := bc.BlockchainState.CurrentMiniRound()
	if err != nil {
		return nil, err
	}

	nextMiniRound, nextRound, err := bc.nextRound(data.MiniRound(currentMiniRound), currentRound)
	if err != nil {
		return nil, err
	}

	proposedBlockHeader := &data.BlockHeader{}
	proposedBlockHeader.Nonce = currentNonce + 1
	proposedBlockHeader.PreviousHash = currentHash
	proposedBlockHeader.Epoch = currentEpoch
	proposedBlockHeader.PreviousRootHash = currentRootHash
	proposedBlockHeader.Round = nextRound
	proposedBlockHeader.MiniRound = uint64(nextMiniRound)

	return proposedBlockHeader, nil
}

func (bc *blockCreator) nextRound(currentMiniRound data.MiniRound, currentRound uint64) (data.MiniRound, uint64, error) {
	switch currentMiniRound {
	case data.MiniRoundOne:
		return data.MiniRoundTwo, currentRound, nil
	case data.MiniRoundTwo:
		return data.MiniRoundThree, currentRound, nil

	case data.MiniRoundThree:
		return data.MiniRoundOne, currentRound + 1, nil

	default:
		return 0, 0, blockprocessing.ErrWrongMiniBlockRound
	}
}

func (bc *blockCreator) createProposedBody() (*data.BlockBody, error) {
	snapshot, err := bc.AccountsSnapshotFactory.CreateSnapshot()
	if err != nil {
		return nil, err
	}
	defer snapshot.Discard()

	txProcessor, err := processor.NewTxProcessor(
		snapshot,
		bc.AccountState,
		bc.Labeler,
		bc.mempool,
	)
	if err != nil {
		return nil, err
	}

	selectedTxs := txProcessor.SelectTransactions()

	proposedBody := &data.BlockBody{
		Transactions: selectedTxs,
	}

	// execute block body, generate labels, validate
	bodyExecutor := blockprocessing.NewBodyExecutor()
	execResult, err := bodyExecutor.ExecuteBlockBody(proposedBody, txProcessor, true)
	if err != nil {
		return nil, err
	}

	proposedBody.Transactions = execResult.Transactions
	proposedBody.Subdomains = execResult.Subdomains
	return proposedBody, nil
}

func (bc *blockCreator) hashProposedBlock(
	proposedBody *data.BlockBody,
	proposedHeader *data.BlockHeader,
) ([]byte, error) {
	bodyHash, err := hashing.ComputeBodyHash(proposedBody)
	if err != nil {
		return nil, err
	}

	proposedHeader.BodyHash = bodyHash

	headerHash, err := hashing.ComputeHeaderHash(proposedHeader)
	if err != nil {
		return nil, err
	}

	return headerHash, nil
}
