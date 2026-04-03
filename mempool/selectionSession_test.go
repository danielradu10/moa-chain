package mempool

import (
	"testing"

	"github.com/stretchr/testify/require"

	"moa-chain/state"
	"moa-chain/testscommon"
)

func newTestSelectionSession(accountsState state.AccountsState) *selectionSession {
	return &selectionSession{
		virtualRecords: make(map[string]*virtualRecord),
		accountsState:  accountsState,
		skippedSenders: make(map[string]struct{}),
	}
}

func TestSelectionSession_OnSelectedTransaction(t *testing.T) {
	t.Parallel()

	t.Run("should create virtual record for first selected transaction", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 30)

		err = session.OnSelectedTransaction(tx)
		require.NoError(t, err)

		vr := session.virtualRecords["alice"]
		require.NotNil(t, vr)
		require.Equal(t, uint64(100), vr.initialBalance)
		require.Equal(t, uint64(30), vr.accumulatedBalance)
		require.Equal(t, uint64(0), vr.currentNonce.Value)
	})

	t.Run("should update virtual record for subsequent selected transaction", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)

		tx1 := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 30)
		tx2 := createTxWithSenderAndValue(1, 10, []byte("txHash2"), "alice", 20)

		err = session.OnSelectedTransaction(tx1)
		require.NoError(t, err)

		err = session.OnSelectedTransaction(tx2)
		require.NoError(t, err)

		vr := session.virtualRecords["alice"]
		require.NotNil(t, vr)
		require.Equal(t, uint64(50), vr.accumulatedBalance)
		require.Equal(t, uint64(1), vr.currentNonce.Value)
	})

	t.Run("should return error when nonce cannot be fetched", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(&testscommon.AccountStateMock{
			GetNonceByAddressCalled: func(address string) (uint64, error) {
				return 0, errExpected
			},
		})

		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 30)

		err := session.OnSelectedTransaction(tx)
		require.ErrorIs(t, err, errExpected)
	})

	t.Run("should return error when balance cannot be fetched", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(&testscommon.AccountStateMock{
			GetNonceByAddressCalled: func(address string) (uint64, error) {
				return 0, nil
			},
			GetBalanceByAddressCalled: func(address string) (uint64, error) {
				return 0, errExpected
			},
		})

		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 30)

		err := session.OnSelectedTransaction(tx)
		require.ErrorIs(t, err, errExpected)
	})
}

func TestSelectionSession_senderShouldBeSkipped(t *testing.T) {
	t.Parallel()

	t.Run("should skip sender when first transaction has initial nonce gap", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.senderShouldBeSkipped(tx)
		require.True(t, shouldSkip)

		_, exists := session.skippedSenders["alice"]
		require.True(t, exists)
	})

	t.Run("should not skip sender when first transaction matches initial nonce", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.senderShouldBeSkipped(tx)
		require.False(t, shouldSkip)
	})

	t.Run("should skip sender already marked as skipped", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.skippedSenders["alice"] = struct{}{}

		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.senderShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should skip sender when nonce is higher than current virtual nonce plus one", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 100, 1)

		tx := createTxWithSenderAndValue(3, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.senderShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should not skip sender when nonce is exactly current virtual nonce plus one", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 100, 1)

		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.senderShouldBeSkipped(tx)
		require.False(t, shouldSkip)
	})
}

func TestSelectionSession_transactionShouldBeSkipped(t *testing.T) {
	t.Parallel()

	t.Run("should skip transaction when sender is already skipped", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.skippedSenders["alice"] = struct{}{}

		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 0)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should skip transaction when nonce is lower than initial nonce", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 3, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 10)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should not skip transaction when nonce matches initial nonce and balance is enough", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 3, 100)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(3, 10, []byte("txHash1"), "alice", 10)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.False(t, shouldSkip)
	})

	t.Run("should skip transaction when nonce is lower than current virtual nonce", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 100, 5)

		tx := createTxWithSenderAndValue(4, 10, []byte("txHash1"), "alice", 10)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should skip transaction when initial balance would be exceeded", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 50)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 60)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should skip transaction when accumulated balance would exceed initial balance", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 1, 50)
		session.virtualRecords["alice"].accumulatedBalance = 40

		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 20)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.True(t, shouldSkip)
	})

	t.Run("should not skip transaction when accumulated balance stays within initial balance", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 100, 1)
		session.virtualRecords["alice"].accumulatedBalance = 40

		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 20)

		shouldSkip := session.transactionShouldBeSkipped(tx)
		require.False(t, shouldSkip)
	})
}

func TestSelectionSession_higherNonceThanInitialNonce(t *testing.T) {
	t.Parallel()

	accountsStateStub := testscommon.NewAccountStateStub()
	err := accountsStateStub.AddAccount("alice", 5, 100)
	require.NoError(t, err)

	session := newTestSelectionSession(accountsStateStub)

	require.False(t, session.higherNonceThanInitialNonce("alice", 5))
	require.True(t, session.higherNonceThanInitialNonce("alice", 6))
}

func TestSelectionSession_lowerOrDuplicatedNonceThanInitialNonce(t *testing.T) {
	t.Parallel()

	accountsStateStub := testscommon.NewAccountStateStub()
	err := accountsStateStub.AddAccount("alice", 5, 100)
	require.NoError(t, err)

	session := newTestSelectionSession(accountsStateStub)

	require.True(t, session.lowerOrDuplicatedNonceThanInitialNonce("alice", 4))
	require.False(t, session.lowerOrDuplicatedNonceThanInitialNonce("alice", 5))
}

func TestSelectionSession_higherNonceThanCurrentNonce(t *testing.T) {
	t.Parallel()

	session := newTestSelectionSession(testscommon.NewAccountStateStub())
	vr := newVirtualRecord(0, 100, 5)

	require.False(t, session.higherNonceThanCurrentNonce(vr, 6))
	require.True(t, session.higherNonceThanCurrentNonce(vr, 7))
}

func TestSelectionSession_lowerOrDuplicatedNonceThanCurrentNonce(t *testing.T) {
	t.Parallel()

	session := newTestSelectionSession(testscommon.NewAccountStateStub())
	vr := newVirtualRecord(0, 100, 5)

	require.True(t, session.lowerOrDuplicatedNonceThanCurrentNonce(vr, 4))
	require.True(t, session.lowerOrDuplicatedNonceThanCurrentNonce(vr, 5))
	require.False(t, session.lowerOrDuplicatedNonceThanCurrentNonce(vr, 6))
}

func TestSelectionSession_initialBalanceWillBeExceeded(t *testing.T) {
	t.Parallel()

	t.Run("should return true when transaction exceeds initial balance and no virtual record exists", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 50)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 60)

		require.True(t, session.initialBalanceWillBeExceeded("alice", tx))
	})

	t.Run("should return false when transaction does not exceed initial balance and no virtual record exists", func(t *testing.T) {
		t.Parallel()

		accountsStateStub := testscommon.NewAccountStateStub()
		err := accountsStateStub.AddAccount("alice", 0, 50)
		require.NoError(t, err)

		session := newTestSelectionSession(accountsStateStub)
		tx := createTxWithSenderAndValue(0, 10, []byte("txHash1"), "alice", 40)

		require.False(t, session.initialBalanceWillBeExceeded("alice", tx))
	})

	t.Run("should return true when accumulated balance exceeds initial balance", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 1, 50)
		session.virtualRecords["alice"].accumulatedBalance = 40

		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 20)

		require.True(t, session.initialBalanceWillBeExceeded("alice", tx))
	})

	t.Run("should return false when accumulated balance does not exceed initial balance", func(t *testing.T) {
		t.Parallel()

		session := newTestSelectionSession(testscommon.NewAccountStateStub())
		session.virtualRecords["alice"] = newVirtualRecord(0, 50, 1)
		session.virtualRecords["alice"].accumulatedBalance = 20

		tx := createTxWithSenderAndValue(2, 10, []byte("txHash1"), "alice", 20)

		require.False(t, session.initialBalanceWillBeExceeded("alice", tx))
	})
}
