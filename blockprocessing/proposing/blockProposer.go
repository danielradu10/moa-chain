package proposing

import (
	"moa-chain/blockprocessing"
	"moa-chain/blockprocessing/hashing"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/transactionprocessing/processor"
)

type blockCreator struct {
	blockprocessing.Base
}

// NewBlockCreator creates a new block creator
func NewBlockCreator(
	base blockprocessing.Base,
) *blockCreator {
	if base.Logger == nil {
		base.Logger = logging.NewNopLogger()
	}

	return &blockCreator{
		Base: base,
	}
}

// ProposeBlockAndDomains proposes a block and the subdomains extracted from the transactions.
func (bc *blockCreator) ProposeBlockAndDomains() (*data.Block, data.Subdomains, []byte, error) {
	bc.Logger.Info("proposing.ProposeBlockAndDomains started")

	proposedHeader, err := bc.createProposedHeader()
	if err != nil {
		bc.Logger.Error("proposing.ProposeBlockAndDomains failed to create proposed block header", "error", err)
		return nil, nil, nil, err
	}
	bc.Logger.Debug(
		"created proposed block header",
		"nonce", proposedHeader.Nonce,
		"round", proposedHeader.Round,
		"miniRound", proposedHeader.MiniRound,
		"epoch", proposedHeader.Epoch,
	)

	proposedBody, subdomains, err := bc.createProposedBodyAndDomains()
	if err != nil {
		bc.Logger.Error("proposing.ProposeBlockAndDomains failed to create proposed block body and domains", "error", err)
		return nil, nil, nil, err
	}
	bc.Logger.Info(
		"proposing.ProposeBlockAndDomains created proposed block body and domains",
		"numTransactions", len(proposedBody.Transactions),
		"numSubdomainMaps", len(subdomains),
	)

	// TODO extract new root hash!! analyze how the account state should behave

	headerHash, err := bc.hashProposedBlock(proposedBody, proposedHeader)
	if err != nil {
		bc.Logger.Error("proposing.ProposeBlockAndDomains failed to hash proposed block", "error", err)
		return nil, nil, nil, err
	}
	proposedHeader.HeaderHash = headerHash
	bc.Logger.Debug("hashed proposed block", "headerHashLen", len(headerHash), "bodyHashLen", len(proposedHeader.BodyHash))

	// return proposed block
	proposedBlock := &data.Block{
		Header: *proposedHeader,
		Body:   *proposedBody,
	}

	subdomainsHash, err := bc.hashSubdomains(subdomains)
	if err != nil {
		bc.Logger.Error("proposing.ProposeBlockAndDomains failed to hash proposed subdomains", "error", err)
		return nil, nil, nil, err
	}
	bc.Logger.Debug("hashed proposed subdomains", "subdomainsHashLen", len(subdomainsHash))

	return proposedBlock, subdomains, subdomainsHash, nil
}

func (bc *blockCreator) createProposedHeader() (*data.BlockHeader, error) {
	// set nonce, round, mini round, epoch, previous header hash, previous root hash
	currentBlock, err := bc.BlockchainState.CurrentBlockHeader()
	if err != nil {
		bc.Logger.Error("failed to load current block header", "error", err)
		return nil, err
	}

	currentNonce := currentBlock.Nonce
	currentHash := currentBlock.HeaderHash
	currentRootHash := currentBlock.RootHash

	currentEpoch, err := bc.BlockchainState.CurrentEpoch()
	if err != nil {
		bc.Logger.Error("failed to load current epoch", "error", err)
		return nil, err
	}

	currentRound, err := bc.BlockchainState.CurrentRound()
	if err != nil {
		bc.Logger.Error("failed to load current round", "error", err)
		return nil, err
	}

	currentMiniRound, err := bc.BlockchainState.CurrentMiniRound()
	if err != nil {
		bc.Logger.Error("failed to load current mini-round", "error", err)
		return nil, err
	}

	nextMiniRound, nextRound, err := bc.nextRound(data.MiniRound(currentMiniRound), currentRound)
	if err != nil {
		bc.Logger.Error("failed to compute next round", "currentRound", currentRound, "currentMiniRound", currentMiniRound, "error", err)
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

func (bc *blockCreator) createProposedBodyAndDomains() (*data.BlockBody, data.Subdomains, error) {
	snapshot, err := bc.AccountsSnapshotFactory.CreateSnapshot()
	if err != nil {
		bc.Logger.Error("failed to create accounts snapshot for proposal", "error", err)
		return nil, nil, err
	}
	defer snapshot.Discard()

	txProcessor, err := processor.NewTxProcessor(
		snapshot,
		bc.AccountState,
		bc.Mempool,
	)
	if err != nil {
		bc.Logger.Error("failed to create transaction processor for proposal", "error", err)
		return nil, nil, err
	}

	selectedTxs := txProcessor.SelectTransactions()
	bc.Logger.Info("proposing.createProposedBodyAndDomains selected transactions for proposal", "numTransactions", len(selectedTxs))

	proposedBody := &data.BlockBody{
		Transactions: selectedTxs,
	}

	bodyExecutor := blockprocessing.NewBodyExecutor(bc.BatchAgent, bc.Logger)
	execResult, err := bodyExecutor.ExecuteBlockBodyMiniRoundOne(proposedBody, txProcessor)
	if err != nil {
		bc.Logger.Error("failed to execute proposed block body", "error", err)
		return nil, nil, err
	}

	proposedBody.Transactions = execResult.Transactions
	return proposedBody, execResult.Subdomains, nil
}

func (bc *blockCreator) hashProposedBlock(
	proposedBody *data.BlockBody,
	proposedHeader *data.BlockHeader,
) ([]byte, error) {
	return hashing.ComputeBlockHash(proposedBody, proposedHeader)
}

func (bc *blockCreator) hashSubdomains(subdomains data.Subdomains) ([]byte, error) {
	return hashing.ComputeSubdomainsHash(subdomains)
}
