package mempool

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/testscommon"
)

var expectedError = errors.New("error expected")

func createTx(nonce uint64, estimatedConsumption uint64, txHash []byte) *transaction {
	return &transaction{
		nonce:                nonce,
		estimatedConsumption: estimatedConsumption,
		txHash:               txHash,
	}
}

func createTxWithSenderAndValue(
	nonce uint64,
	estimatedConsumption uint64,
	txHash []byte,
	sender string,
	transferredValue uint64,
) *transaction {
	tx := createTx(nonce, estimatedConsumption, txHash)
	tx.sender = []byte(sender)
	tx.transferredValue = transferredValue

	return tx
}

func createSelectionTx(
	nonce uint64,
	sender string,
	estimatedScore uint64,
	estimatedConsumption uint64,
	transferredValue uint64,
	txHash []byte,
) *transaction {
	return &transaction{
		nonce:                nonce,
		sender:               []byte(sender),
		estimatedScore:       estimatedScore,
		estimatedConsumption: estimatedConsumption,
		transferredValue:     transferredValue,
		txHash:               txHash,
	}
}

func createAccountsStateWithAccounts(t *testing.T, accounts map[string]struct {
	nonce   uint64
	balance uint64
}) *testscommon.AccountStateStub {
	t.Helper()

	accountsStateStub := testscommon.NewAccountStateStub()
	for address, account := range accounts {
		err := accountsStateStub.AddAccount(address, account.nonce, account.balance)
		require.NoError(t, err)
	}

	return accountsStateStub
}
