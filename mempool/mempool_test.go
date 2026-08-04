package mempool

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/data"
)

func TestMemPool_AddTransaction(t *testing.T) {
	t.Parallel()

	t.Run("should return ErrNilTransaction in case of nil transaction", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		err := mempool.AddTransaction(nil)
		require.Equal(t, ErrNilTransaction, err)
	})

	t.Run("should add transaction", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		err := mempool.AddTransaction(createPoolTx(0, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)
		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())
	})

	t.Run("should add two transactions from same sender and keep one address", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		err := mempool.AddTransaction(createPoolTx(1, "alice", 20, []byte("txHash2")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(0, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)

		require.Equal(t, uint64(2), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, uint64(0), aliceTxList.getTxByIndex(0).GetNonce())
		require.Equal(t, uint64(1), aliceTxList.getTxByIndex(1).GetNonce())
	})

	t.Run("should add transactions from different senders", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		err := mempool.AddTransaction(createPoolTx(0, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(0, "bob", 15, []byte("txHash2")))
		require.NoError(t, err)

		require.Equal(t, uint64(2), mempool.NumTransactions())
		require.Equal(t, uint64(2), mempool.NumAddresses())
	})

	t.Run("should not overwrite transaction by hash in case of duplicate tx hash", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		tx1 := createPoolTx(0, "alice", 10, []byte("sameHash"))
		tx2 := createPoolTx(1, "bob", 20, []byte("sameHash"))

		err := mempool.AddTransaction(tx1)
		require.NoError(t, err)

		err = mempool.AddTransaction(tx2)
		require.NoError(t, err)

		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, tx1, mempool.transactionsByHash["sameHash"])
	})

	t.Run("should not add transactions to sender list when tx hash is duplicate", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		tx1 := createPoolTx(1, "alice", 10, []byte("sameHash"))
		tx2 := createPoolTx(1, "alice", 10, []byte("sameHash"))

		err := mempool.AddTransaction(tx1)
		require.NoError(t, err)

		err = mempool.AddTransaction(tx2)
		require.NoError(t, err)

		require.Equal(t, uint64(1), mempool.NumTransactions())
		require.Equal(t, uint64(1), mempool.NumAddresses())

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 1, len(aliceTxList.transactionList))
	})

	t.Run("should keep sender transactions sorted", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		err := mempool.AddTransaction(createPoolTx(2, "alice", 30, []byte("txHash3")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(1, "alice", 20, []byte("txHash2")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(0, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 3, aliceTxList.numTransactions())
		require.Equal(t, uint64(0), aliceTxList.getTxByIndex(0).GetNonce())
		require.Equal(t, uint64(1), aliceTxList.getTxByIndex(1).GetNonce())
		require.Equal(t, uint64(2), aliceTxList.getTxByIndex(2).GetNonce())
	})

	t.Run("should sort same nonce transactions by estimated consumption", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		err := mempool.AddTransaction(createPoolTx(1, "alice", 30, []byte("txHash2")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(1, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, uint64(10), aliceTxList.getTxByIndex(0).GetEstimatedConsumption())
		require.Equal(t, uint64(30), aliceTxList.getTxByIndex(1).GetEstimatedConsumption())
	})

	t.Run("should sort same nonce and same estimated consumption by tx hash", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()

		err := mempool.AddTransaction(createPoolTx(1, "alice", 10, []byte("txHash2")))
		require.NoError(t, err)

		err = mempool.AddTransaction(createPoolTx(1, "alice", 10, []byte("txHash1")))
		require.NoError(t, err)

		aliceTxList, err := mempool.getTransactionsListBySender([]byte("alice"))
		require.NoError(t, err)

		require.Equal(t, 2, aliceTxList.numTransactions())
		require.Equal(t, []byte("txHash1"), aliceTxList.getTxByIndex(0).GetTxHash())
		require.Equal(t, []byte("txHash2"), aliceTxList.getTxByIndex(1).GetTxHash())
	})
}

func TestMemPool_SelectTransactions(t *testing.T) {
	t.Parallel()

	t.Run("should return empty list when mempool is empty", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{})

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Empty(t, selectedTransactions)
	})

	t.Run("should select transactions in comparator order across senders", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
			"carol": {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 50, 30, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 80, 20, 10, []byte("txHash2"))
		tx3 := createSelectionTx(0, "carol", 60, 10, 10, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx2, tx3, tx1}, selectedTransactions)
	})

	t.Run("should use estimated consumption as tie breaker when scores are equal", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 30, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 100, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx2, tx1}, selectedTransactions)
	})

	t.Run("should use tx hash as tie breaker when score and consumption are equal", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 100, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx1, tx2}, selectedTransactions)
	})

	t.Run("should stop when next best transaction exceeds max block consumption", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := &transaction{nonce: 0, sender: []byte("alice"), txHash: []byte("txHash1"), tip: 900000, estimatedConsumption: 9000, estimatedScore: 100, transferredValue: 10}
		tx2 := &transaction{nonce: 0, sender: []byte("bob"), txHash: []byte("txHash2"), tip: 180000, estimatedConsumption: 2000, estimatedScore: 90, transferredValue: 10}

		addTxDirect(mempool, tx1)
		addTxDirect(mempool, tx2)

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx1}, selectedTransactions)
	})

	t.Run("should stop when max number of transactions is reached", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{})

		for i := 0; i < maxNumTransactions+5; i++ {
			sender := fmt.Sprintf("sender-%d", i)
			require.NoError(t, accountsState.AddAccount(sender, 0, 100000))
			tx := createSelectionTx(0, sender, 10, 10, 10, []byte(fmt.Sprintf("hash-%d", i)))
			require.NoError(t, mempool.AddTransaction(tx))
		}

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Len(t, selectedTransactions, maxNumTransactions)
	})

	t.Run("should skip sender when first transaction has initial nonce gap", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(2, "alice", 100, 10, 10, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 50, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx2}, selectedTransactions)
	})

	t.Run("should skip transaction when balance is insufficient", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 50},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 60, []byte("txHash1"))
		tx2 := createSelectionTx(0, "bob", 50, 10, 10, []byte("txHash2"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx2}, selectedTransactions)
	})

	t.Run("should select multiple valid consecutive transactions from same sender", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 100},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 20, []byte("txHash1"))
		tx2 := createSelectionTx(1, "alice", 90, 10, 20, []byte("txHash2"))
		tx3 := createSelectionTx(0, "bob", 80, 10, 20, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx1, tx2, tx3}, selectedTransactions)
	})

	t.Run("should skip second transaction from sender when accumulated transferred value exceeds balance", func(t *testing.T) {
		t.Parallel()

		mempool := newTestMemPool()
		accountsState := createAccountsStateWithAccounts(t, map[string]struct {
			nonce   uint64
			balance uint64
		}{
			"alice": {nonce: 0, balance: 50},
			"bob":   {nonce: 0, balance: 100},
		})

		tx1 := createSelectionTx(0, "alice", 100, 10, 30, []byte("txHash1"))
		tx2 := createSelectionTx(1, "alice", 90, 10, 30, []byte("txHash2"))
		tx3 := createSelectionTx(0, "bob", 80, 10, 10, []byte("txHash3"))

		require.NoError(t, mempool.AddTransaction(tx1))
		require.NoError(t, mempool.AddTransaction(tx2))
		require.NoError(t, mempool.AddTransaction(tx3))

		selectedTransactions := mempool.SelectTransactions(accountsState)

		require.Equal(t, []data.Transaction{tx1, tx3}, selectedTransactions)
	})
}

func TestMemPool_calculateNumTokensFromPrompt(t *testing.T) {
	t.Parallel()

	mp := newTestMemPool()
	prompt := "Summarize the following pull request in three bullets and highlight any risk around transaction ordering."
	tx := &transaction{prompt: []byte(prompt)}

	expectedTokens := countPromptTokensForTest(t, prompt)

	calculatedTokens, err := mp.calculateNumTokensFromPrompt(tx)
	require.NoError(t, err)

	require.Equal(t, expectedTokens, calculatedTokens)
}

func TestMemPool_calculateNumTokensFromPromptShouldIncreaseForLongerRealPrompt(t *testing.T) {
	t.Parallel()

	mp := newTestMemPool()
	shortPrompt := "Write a short summary of this transaction."
	longPrompt := "Write a short summary of this transaction, explain whether the sender nonce is valid, and list the likely reasons the transaction could be skipped during block selection."

	shortTx := &transaction{prompt: []byte(shortPrompt)}
	longTx := &transaction{prompt: []byte(longPrompt)}

	longRes, err := mp.calculateNumTokensFromPrompt(longTx)
	require.NoError(t, err)

	shortRes, err := mp.calculateNumTokensFromPrompt(shortTx)
	require.NoError(t, err)

	require.Greater(t, longRes, shortRes)
}

func TestMemPool_estimateNumTokens(t *testing.T) {
	t.Parallel()

	mp := newTestMemPool()
	prompt := "Generate a concise explanation of why this AI request should be prioritized in the block."
	tx := &transaction{
		prompt:              []byte(prompt),
		userOutputDimension: "medium",
		thinkingMode:        "standard",
	}

	expectedTokens := countPromptTokensForTest(t, prompt) + mp.tokensConfig["medium"] + mp.tokensConfig["standard"]

	actualRes, err := mp.estimateNumTokens(tx)
	require.NoError(t, err)
	require.Equal(t, expectedTokens, actualRes)
}

func TestMemPool_precomputeTxFields(t *testing.T) {
	t.Parallel()

	mp := newTestMemPool()
	prompt := "Read the prompt carefully, plan the response, and produce a detailed answer about mempool fairness."
	tx := &transaction{
		prompt:              []byte(prompt),
		tip:                 500,
		userOutputDimension: "long",
		thinkingMode:        "thinking",
	}

	err := mp.precomputeTxFields(tx)
	require.NoError(t, err)

	expectedConsumption := countPromptTokensForTest(t, prompt) + mp.tokensConfig["long"] + mp.tokensConfig["thinking"]
	require.Equal(t, expectedConsumption, tx.GetEstimatedConsumption())
	require.Equal(t, tx.GetTip()/expectedConsumption, tx.GetEstimatedScore())
}

func TestMemPool_AddTransactionShouldPrecomputeFields(t *testing.T) {
	t.Parallel()

	mp := newTestMemPool()
	prompt := "Classify this request, estimate the complexity, and return a medium-length answer."
	tx := &transaction{
		nonce:                0,
		prompt:               []byte(prompt),
		sender:               []byte("alice"),
		tip:                  700,
		txHash:               []byte("txHash1"),
		userOutputDimension:  "medium",
		thinkingMode:         "fast",
		estimatedConsumption: 999,
		estimatedScore:       999,
	}

	err := mp.AddTransaction(tx)

	require.NoError(t, err)
	expectedConsumption := countPromptTokensForTest(t, prompt) + mp.tokensConfig["medium"] + mp.tokensConfig["fast"]
	require.Equal(t, expectedConsumption, tx.GetEstimatedConsumption())
	require.Equal(t, tx.GetTip()/expectedConsumption, tx.GetEstimatedScore())
}
