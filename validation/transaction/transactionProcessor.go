package transaction

import (
	"moa-chain/data"
	"moa-chain/state"
	"moa-chain/validation"
)

type txProcessor struct {
	accountsProvider state.AccountsProvider
}

func (tp *txProcessor) ProcessTransaction(tx data.Transaction) error {
	sender := tx.GetSender()
	receiver := tx.GetReceiver()

	senderAccount, err := tp.accountsProvider.LoadAccount(string(sender))
	if err != nil {
		return err
	}

	receiverAccount, err := tp.accountsProvider.LoadAccount(string(receiver))
	if err != nil {
		return err
	}

	// validate
	err = tp.validateTransaction(tx, senderAccount)
	if err != nil {
		return err
	}

	// process (but in a simulated account state)
	err = tp.processTransaction(tx, receiverAccount, senderAccount)
	if err != nil {
		return err
	}

	return nil
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
	txBalance := tx.GetTransferredValue() + tx.GetEstimatedFee() + tx.GetTip()
	if txBalance > senderBalance {
		return validation.ErrWrongTransactionBalance
	}

	return nil
}

func (tp *txProcessor) processTransaction(
	tx data.Transaction,
	senderAccount state.AccountHandler,
	receiverAccount state.AccountHandler,
) error {
	// process transaction
	err := senderAccount.IncreaseNonce(1)
	if err != nil {
		return err
	}

	// TODO analyze if this is the same transferred balance calculated in mempool
	txTransferredValue := tx.GetTransferredValue() + tx.GetEstimatedFee() + tx.GetNonce()
	err = senderAccount.DecreaseBalance(txTransferredValue)
	if err != nil {
		return err
	}

	// TODO analyze if this is needed for the moment
	// where should the transferred value of a prompt go?
	err = receiverAccount.IncreaseBalance(txTransferredValue)
	if err != nil {
		return err
	}

	return nil
}
