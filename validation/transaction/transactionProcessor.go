package transaction

import (
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validation"
)

type txProcessor struct {
	accountsProvider state.AccountsProvider
}

func NewTxProcessor(accountsProvider state.AccountsProvider) (*txProcessor, error) {
	return &txProcessor{
		accountsProvider: accountsProvider,
	}, nil
}

func (tp *txProcessor) ProcessTransaction(tx data.Transaction, miniRound validation.MiniRound) (uint64, error) {
	sender := tx.GetSender()

	senderAccount, err := tp.accountsProvider.LoadAccount(string(sender))
	if err != nil {
		return 0, err
	}

	escrowAccount, err := tp.accountsProvider.LoadEscrowAccount()
	if err != nil {
		return 0, err
	}

	// validate the transaction
	err = tp.validateTransaction(tx, senderAccount)
	if err != nil {
		return 0, err
	}

	switch miniRound {
	case validation.MiniRoundOne:
		return tx.GetEstimatedConsumption(), tp.reserveTransaction(tx, senderAccount, escrowAccount)
	case validation.MiniRoundTwo:
		return 0, validation.ErrNotImplemented
	case validation.MiniRoundThree:
		return 0, validation.ErrNotImplemented
	default:
		return 0, validation.ErrUnsupportedMiniRound
	}
}

func (tp *txProcessor) validateTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
) error {
	// check if the nonce of the transaction corresponds with the nonce of the account
	txNonce := tx.GetNonce()
	senderNonce, err := senderAccount.Nonce()
	if err != nil {
		return err
	}

	if txNonce != senderNonce {
		return validation.ErrWrongTransactionNonce
	}

	// check if the account has enough balance
	senderBalance, err := senderAccount.Balance()
	if err != nil {
		return err
	}

	// TODO analyze if this is the same transferred balance calculated in mempool
	txBalance := tp.computeReservedBudget(tx)
	if txBalance > senderBalance {
		return validation.ErrWrongTransactionBalance
	}

	return nil
}

func (tp *txProcessor) reserveTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
	escrowAccount state.AccountHandler,
) error {
	// process transaction
	err := senderAccount.IncreaseNonce(1)
	if err != nil {
		return err
	}

	// TODO analyze if this is the same transferred balance calculated in mempool
	txTransferredValue := tp.computeReservedBudget(tx)
	err = senderAccount.DecreaseBalance(txTransferredValue)
	if err != nil {
		return err
	}

	err = escrowAccount.IncreaseBalance(txTransferredValue)
	if err != nil {
		return err
	}

	return nil
}

func (tp *txProcessor) computeReservedBudget(tx data.Transaction) uint64 {
	return tx.GetEstimatedFee() + tx.GetTip()
}
