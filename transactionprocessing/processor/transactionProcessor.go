package processor

import (
	"bytes"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/state"
	"moa-chain/transactionprocessing"
)

type txProcessor struct {
	accountsProvider state.AccountsProvider
	accountState     state.AccountsState
	mempool          mempool.Mempool
	labeler          agent.Agent
}

func NewTxProcessor(
	accountsProvider state.AccountsProvider,
	accountState state.AccountsState,
	labeler agent.Agent,
	mempool mempool.Mempool,
) (*txProcessor, error) {
	return &txProcessor{
		accountsProvider: accountsProvider,
		accountState:     accountState,
		labeler:          labeler,
		mempool:          mempool,
	}, nil
}

func (tp *txProcessor) ProcessTransactionEconomically(tx data.Transaction, miniRound data.MiniRound) (uint64, error) {
	sender := tx.GetSender()

	senderAccount, err := tp.accountsProvider.LoadAccount(string(sender))
	if err != nil {
		return 0, err
	}

	escrowAccount, err := tp.accountsProvider.LoadEscrowAccount()
	if err != nil {
		return 0, err
	}

	err = tp.validateTransaction(tx, senderAccount)
	if err != nil {
		return 0, err
	}

	switch miniRound {
	case data.MiniRoundOne:
		return tx.GetEstimatedConsumption(), tp.reserveTransaction(tx, senderAccount, escrowAccount)
	case data.MiniRoundTwo:
		return 0, transactionprocessing.ErrNotImplemented
	case data.MiniRoundThree:
		return 0, transactionprocessing.ErrNotImplemented
	default:
		return 0, transactionprocessing.ErrUnsupportedMiniRound
	}
}

func (tp *txProcessor) validateTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	err := tp.validateTransactionNonce(tx, senderAccount)
	if err != nil {
		return err
	}

	err = tp.validateTransactionBalance(tx, senderAccount)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) validateTransactionNonce(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	txNonce := tx.GetNonce()

	senderNonce, err := senderAccount.Nonce()
	if err != nil {
		return err
	}

	if txNonce != senderNonce {
		return transactionprocessing.ErrWrongTransactionNonce
	}

	return nil
}

func (tp *txProcessor) validateTransactionBalance(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	senderBalance, err := senderAccount.Balance()
	if err != nil {
		return err
	}

	reservedBudget := tp.computeReservedBudget(tx)
	if reservedBudget > senderBalance {
		return transactionprocessing.ErrWrongTransactionBalance
	}

	return nil
}

func (tp *txProcessor) LabelTransaction(tx data.Transaction) ([]string, error) {
	return tp.labeler.Label(tx)
}

func (tp *txProcessor) computeReservedBudget(tx data.Transaction) uint64 {
	return tx.GetEstimatedFee() + tx.GetTip()
}

func (tp *txProcessor) reserveTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
	escrowAccount state.AccountHandler,
) error {
	err := senderAccount.IncreaseNonce(1)
	if err != nil {
		return err
	}

	reservedBudget := tp.computeReservedBudget(tx)

	err = senderAccount.DecreaseBalance(reservedBudget)
	if err != nil {
		return err
	}

	err = escrowAccount.IncreaseBalance(reservedBudget)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) ValidateTransactionsOrdering(
	previousTransaction data.Transaction,
	currentTransaction data.Transaction,
) error {
	prevScore := previousTransaction.GetEstimatedScore()
	currScore := currentTransaction.GetEstimatedScore()

	if prevScore < currScore {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	if prevScore > currScore {
		return nil
	}

	prevConsumption := previousTransaction.GetEstimatedConsumption()
	currConsumption := currentTransaction.GetEstimatedConsumption()

	if prevConsumption > currConsumption {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	if prevConsumption < currConsumption {
		return nil
	}

	if bytes.Compare(previousTransaction.GetTxHash(), currentTransaction.GetTxHash()) > 0 {
		return transactionprocessing.ErrTxsDoNotRespectProtocolOrder
	}

	return nil
}

func (tp *txProcessor) SelectTransactions() []data.Transaction {
	selectedTxs := tp.mempool.SelectTransactions(tp.accountState)
	return selectedTxs
}
