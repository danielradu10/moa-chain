package processor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
	"moa-chain/mempool"
	"moa-chain/testscommon"
	"moa-chain/transactionprocessing"
)

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

		txProcessor, err := NewTxProcessor(
			accountsProvider,
			createAccountsStateStub(t),
			labeler,
			mempool.NewMemPool(),
		)
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			sender: "alice",
		})

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
		require.NoError(t, err)

		tx := createTestTransaction(testTransactionArgs{
			sender: "alice",
		})

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, transactionprocessing.ErrWrongTransactionNonce, err)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, transactionprocessing.ErrWrongTransactionBalance, err)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundTwo)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, transactionprocessing.ErrNotImplemented, err)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundThree)
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, transactionprocessing.ErrNotImplemented, err)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRound(99))
		require.Equal(t, uint64(0), estimatedConsumption)
		require.Equal(t, transactionprocessing.ErrUnsupportedMiniRound, err)
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

		txProcessor, err := NewTxProcessor(accountsProvider, createAccountsStateStub(t), labeler, mempool.NewMemPool())
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

		estimatedConsumption, err := txProcessor.ProcessTransactionEconomically(tx, data.MiniRoundOne)
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

func TestPromptExecutor_ExecutePromptTransaction(t *testing.T) {
	t.Parallel()

	t.Run("should return answer with actual consumption", func(t *testing.T) {
		t.Parallel()

		tx := createTestTransaction(testTransactionArgs{
			txHash: "txHash1",
		})
		answer := "deterministic validator answer"
		labeler := &testscommon.LabelerStub{
			AnswersByTxHash: map[string]string{
				"txHash1": answer,
			},
		}
		promptExecutor, err := NewPromptExecutor(labeler)
		require.NoError(t, err)

		expectedConsumption, err := calculateNumTokensFromPrompt(answer)
		require.NoError(t, err)

		result, err := promptExecutor.ExecutePromptTransaction(tx)

		require.NoError(t, err)
		require.Equal(t, []byte("txHash1"), result.TxHash)
		require.Equal(t, answer, result.Answer)
		require.Equal(t, expectedConsumption, result.ActualConsumption)
		require.Greater(t, result.ActualConsumption, uint64(0))
	})

	t.Run("should return answer generation error", func(t *testing.T) {
		t.Parallel()

		expectedErr := errors.New("answer generation error")
		tx := createTestTransaction(testTransactionArgs{
			txHash: "txHash1",
		})
		promptExecutor, err := NewPromptExecutor(
			&testscommon.LabelerStub{
				AnswerErr: expectedErr,
			},
		)
		require.NoError(t, err)

		result, err := promptExecutor.ExecutePromptTransaction(tx)

		require.Nil(t, result)
		require.Equal(t, expectedErr, err)
	})

	t.Run("should return ErrNilAgent when agent is nil", func(t *testing.T) {
		t.Parallel()

		promptExecutor, err := NewPromptExecutor(nil)

		require.Nil(t, promptExecutor)
		require.Equal(t, transactionprocessing.ErrNilAgent, err)
	})
}

func TestBlockProcessor_validateTransactionsOrdering(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrTxsDoNotRespectProtocolOrder when score increases", func(t *testing.T) {
		t.Parallel()

		txProc := &txProcessor{}

		previousTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})
		currentTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       110,
			estimatedConsumption: 20,
		})

		err := txProc.ValidateTransactionsOrdering(previousTx, currentTx)

		require.Equal(t, transactionprocessing.ErrTxsDoNotRespectProtocolOrder, err)
	})

	t.Run("should return ErrTxsDoNotRespectProtocolOrder when score is equal and consumption decreases", func(t *testing.T) {
		t.Parallel()

		txProc := &txProcessor{}

		previousTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})
		currentTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       100,
			estimatedConsumption: 10,
		})

		err := txProc.ValidateTransactionsOrdering(previousTx, currentTx)

		require.Equal(t, transactionprocessing.ErrTxsDoNotRespectProtocolOrder, err)
	})

	t.Run("should return ErrTxsDoNotRespectProtocolOrder when score and consumption are equal and hash is out of order", func(t *testing.T) {
		t.Parallel()

		txProc := &txProcessor{}

		previousTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})
		currentTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})

		err := txProc.ValidateTransactionsOrdering(previousTx, currentTx)

		require.Equal(t, transactionprocessing.ErrTxsDoNotRespectProtocolOrder, err)
	})

	t.Run("should validate transactions ordering when transactions are correctly ordered", func(t *testing.T) {
		t.Parallel()

		txProc := &txProcessor{}

		previousTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash1",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})
		currentTx := createTestTransaction(testTransactionArgs{
			txHash:               "txHash2",
			estimatedScore:       100,
			estimatedConsumption: 20,
		})

		err := txProc.ValidateTransactionsOrdering(previousTx, currentTx)

		require.NoError(t, err)
	})
}

type testTransactionArgs struct {
	nonce                uint64
	sender               string
	txHash               string
	estimatedScore       uint64
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
	ts.SetEstimatedScore(args.estimatedScore)

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

func createAccountsStateStub(t *testing.T) *testscommon.AccountStateStub {
	t.Helper()
	return &testscommon.AccountStateStub{}
}
