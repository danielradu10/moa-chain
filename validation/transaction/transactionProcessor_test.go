package transaction

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/testscommon"
	"moa-chain/validation"
)

func TestTxProcessor_validateLabels(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrLeaderGeneratedTooManyLabels when leader proposes too many labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"cloud_engineering",
				"databases",
				"mobile_dev",
			},
			[]string{
				"security",
				"cloud_engineering",
				"databases",
				"mobile_dev",
			},
		)

		require.Equal(t, validation.ErrLeaderGeneratedTooManyLabels, err)
	})

	t.Run("should return ErrValidatorGeneratedTooManyLabels when validator generates too many labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"cloud_engineering",
				"databases",
			},
			[]string{
				"security",
				"cloud_engineering",
				"databases",
				"mobile_dev",
				"dev_ops",
				"systems_programming",
				"blockchain_engineering",
			},
		)

		require.Equal(t, validation.ErrValidatorGeneratedTooManyLabels, err)
	})

	t.Run("should return ErrUnknownLabel when leader proposes an unknown label", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"unknown_label",
			},
			[]string{
				"security",
				"cloud_engineering",
				"databases",
			},
		)

		require.Equal(t, validation.ErrUnknownLabel, err)
	})

	t.Run("should return ErrUnknownLabel when validator generates an unknown label", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
			},
			[]string{
				"security",
				"unknown_label",
			},
		)

		require.Equal(t, validation.ErrUnknownLabel, err)
	})

	t.Run("should return ErrLeaderProposedDuplicatedLabels when leader proposes duplicated labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"security",
			},
			[]string{
				"security",
				"cloud_engineering",
				"databases",
			},
		)

		require.Equal(t, validation.ErrLeaderProposedDuplicatedLabels, err)
	})

	t.Run("should return ErrValidatorGeneratedDuplicatedLabels when validator generates duplicated labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
			},
			[]string{
				"security",
				"security",
			},
		)

		require.Equal(t, validation.ErrValidatorGeneratedDuplicatedLabels, err)
	})

	t.Run("should return ErrLabelIsNotValid when leader labels are not contained in validator labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"cloud_engineering",
				"databases",
			},
			[]string{
				"security",
				"mobile_dev",
				"databases",
				"back_end_with_apis",
			},
		)

		require.Equal(t, validation.ErrLabelIsNotValid, err)
	})

	t.Run("should validate labels when leader labels are contained in validator labels", func(t *testing.T) {
		t.Parallel()

		txProcessor := &txProcessor{}

		err := txProcessor.validateLabels(
			[]string{
				"security",
				"cloud_engineering",
				"databases",
			},
			[]string{
				"security",
				"cloud_engineering",
				"databases",
				"mobile_dev",
				"dev_ops",
				"back_end_with_apis",
			},
		)

		require.NoError(t, err)
	})
}

func TestTxProcessor_computeReservedBudget(t *testing.T) {
	t.Parallel()

	txProcessor := &txProcessor{}
	tx := createTestTransaction(testTransactionArgs{
		estimatedFee: 25,
		tip:          10,
	})

	require.Equal(t, uint64(35), txProcessor.computeReservedBudget(tx))
}

func TestTxProcessor_reserveTransaction(t *testing.T) {
	t.Parallel()

	txProcessor := &txProcessor{}

	senderAccount := testscommon.NewAccountHandlerStub(7, 100)
	escrowAccount := testscommon.NewAccountHandlerStub(0, 20)

	tx := createTestTransaction(testTransactionArgs{
		estimatedFee: 15,
		tip:          5,
	})

	err := txProcessor.reserveTransaction(tx, senderAccount, escrowAccount)
	require.NoError(t, err)

	senderNonce, err := senderAccount.Nonce()
	require.NoError(t, err)
	require.Equal(t, uint64(8), senderNonce)

	senderBalance, err := senderAccount.Balance()
	require.NoError(t, err)
	require.Equal(t, uint64(80), senderBalance)

	escrowBalance, err := escrowAccount.Balance()
	require.NoError(t, err)
	require.Equal(t, uint64(40), escrowBalance)
}

func TestTxProcessor_ProcessTransaction(t *testing.T) {
	t.Parallel()

	t.Run("should return load account error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("load account error")
		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			loadAccountErr: expectedErr,
		})
		labeler := &testscommon.LabelerStub{}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			sender: "alice",
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should return load escrow account error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("load escrow error")

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState:  accountState,
			addresses:     []string{"alice"},
			loadEscrowErr: expectedErr,
		})
		labeler := &testscommon.LabelerStub{}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			sender: "alice",
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should return labeler error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("labeler error")

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			Err: expectedErr,
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should return ErrWrongTransactionNonce when transaction nonce does not match sender nonce", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 2, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {"security", "cloud_engineering"},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                1,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, validation.ErrWrongTransactionNonce, err)
	})

	t.Run("should return ErrWrongTransactionBalance when sender does not have enough balance", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 10},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {"security", "cloud_engineering"},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         8,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, validation.ErrWrongTransactionBalance, err)
	})

	t.Run("should return ErrNotImplemented in mini round two", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {"security", "cloud_engineering"},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundTwo)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, validation.ErrNotImplemented, err)
	})

	t.Run("should return ErrNotImplemented in mini round three", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {"security", "cloud_engineering"},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundThree)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, validation.ErrNotImplemented, err)
	})

	t.Run("should return ErrUnsupportedMiniRound for unknown mini round", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState: accountState,
			addresses:    []string{"alice"},
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {"security", "cloud_engineering"},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 70,
			domainLabels:         []string{"security"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRound(99))
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, validation.ErrUnsupportedMiniRound, err)
	})

	t.Run("should reserve transaction and return estimated consumption in mini round one", func(t *testing.T) {
		t.Parallel()

		accountState := createAccountStateStubWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
		})

		accountsProvider := createAccountsProviderWithErrors(t, testAccountsProviderArgs{
			accountState:  accountState,
			addresses:     []string{"alice"},
			escrowBalance: 20,
		})
		labeler := &testscommon.LabelerStub{
			LabelsByTxHash: map[string][]string{
				"txHash1": {
					"security",
					"cloud_engineering",
					"databases",
					"dev_ops",
				},
			},
		}

		txProcessor, err := NewTxProcessor(accountsProvider, labeler)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			nonce:                0,
			sender:               "alice",
			txHash:               "txHash1",
			estimatedFee:         10,
			tip:                  5,
			estimatedConsumption: 77,
			domainLabels:         []string{"security", "cloud_engineering", "databases"},
		})

		estimatedConsumption, err := txProcessor.ProcessTransaction(tx, validation.MiniRoundOne)
		require.NoError(t, err)
		require.Equal(t, uint64(77), estimatedConsumption)

		aliceAccount, err := accountsProvider.LoadAccount("alice")
		require.NoError(t, err)

		escrowAccount, err := accountsProvider.LoadEscrowAccount()
		require.NoError(t, err)

		aliceNonce, err := aliceAccount.Nonce()
		require.NoError(t, err)
		require.Equal(t, uint64(1), aliceNonce)

		aliceBalance, err := aliceAccount.Balance()
		require.NoError(t, err)
		require.Equal(t, uint64(85), aliceBalance)

		escrowBalance, err := escrowAccount.Balance()
		require.NoError(t, err)
		require.Equal(t, uint64(35), escrowBalance)
	})
}

type testTransactionArgs struct {
	nonce                uint64
	sender               string
	txHash               string
	estimatedConsumption uint64
	estimatedFee         uint64
	tip                  uint64
	domainLabels         []string
}

func createTestTransaction(args testTransactionArgs) *testscommon.TransactionStub {
	ts := &testscommon.TransactionStub{}
	ts.SetNonce(args.nonce)
	ts.SetSender([]byte(args.sender))
	ts.SetTxHash([]byte(args.txHash))
	ts.SetEstimatedConsumption(args.estimatedConsumption)
	ts.SetEstimatedFee(args.estimatedFee)
	ts.SetTip(args.tip)
	ts.SetDomainLabels(args.domainLabels)

	return ts
}

type testAccountsProviderArgs struct {
	accountState   *testscommon.AccountStateStub
	addresses      []string
	escrowBalance  uint64
	loadAccountErr error
	loadEscrowErr  error
}

func createAccountStateStubWithAccounts(
	t *testing.T,
	accounts map[string]struct {
		nonce   uint64
		balance uint64
	},
) *testscommon.AccountStateStub {
	t.Helper()

	accountState := testscommon.NewAccountStateStub()
	for address, accountData := range accounts {
		err := accountState.AddAccount(address, accountData.nonce, accountData.balance)
		require.NoError(t, err)
	}

	return accountState
}

func createAccountsProviderWithErrors(t *testing.T, args testAccountsProviderArgs) *testscommon.AccountsProviderStub {
	t.Helper()

	accounts := make(map[string]*testscommon.AccountHandlerStub, len(args.addresses))
	for _, address := range args.addresses {
		require.NotNil(t, args.accountState)

		nonce, err := args.accountState.GetNonceByAddress(address)
		require.NoError(t, err)

		balance, err := args.accountState.GetBalanceByAddress(address)
		require.NoError(t, err)

		accounts[address] = testscommon.NewAccountHandlerStub(nonce, balance)
	}

	return &testscommon.AccountsProviderStub{
		Accounts:       accounts,
		EscrowAccount:  testscommon.NewAccountHandlerStub(0, args.escrowBalance),
		LoadAccountErr: args.loadAccountErr,
		LoadEscrowErr:  args.loadEscrowErr,
	}
}
