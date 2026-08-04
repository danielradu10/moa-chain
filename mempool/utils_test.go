package mempool

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tiktoken-go/tokenizer"

	"moa-chain/testscommon"
)

var errExpected = errors.New("error expected")

func newTestMemPool() *memPool {
	mp := NewMemPool()
	mp.tokensConfig = map[string]uint64{
		"short":         0,
		"medium":        10,
		"long":          15,
		"fast":          10,
		"standard":      15,
		"thinking":      1985,
		"deep research": 8985,
	}

	return mp
}

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
		nonce:               nonce,
		prompt:              []byte{},
		sender:              []byte(sender),
		transferredValue:    transferredValue,
		tip:                 estimatedScore * estimatedConsumption,
		txHash:              txHash,
		userOutputDimension: consumptionToOutputDimension(estimatedConsumption),
		thinkingMode:        consumptionToThinkingMode(estimatedConsumption),
	}
}

func createPoolTx(
	nonce uint64,
	sender string,
	estimatedConsumption uint64,
	txHash []byte,
) *transaction {
	return &transaction{
		nonce:               nonce,
		prompt:              []byte{},
		sender:              []byte(sender),
		txHash:              txHash,
		userOutputDimension: consumptionToOutputDimension(estimatedConsumption),
		thinkingMode:        consumptionToThinkingMode(estimatedConsumption),
	}
}

func consumptionToOutputDimension(estimatedConsumption uint64) string {
	switch estimatedConsumption {
	case 10:
		return "short"
	case 20:
		return "medium"
	case 15, 30, 2000, 9000:
		return "long"
	default:
		return "short"
	}
}

func consumptionToThinkingMode(estimatedConsumption uint64) string {
	switch estimatedConsumption {
	case 10, 20, 25:
		return "fast"
	case 15, 30:
		return "standard"
	case 2000:
		return "thinking"
	case 9000:
		return "deep research"
	default:
		return "fast"
	}
}

// addTxDirect inserts a transaction into the mempool without running the precompute
// step. Use this in tests that need full control over estimatedConsumption.
func addTxDirect(mp *memPool, tx *transaction) {
	mp.mempoolMutex.Lock()
	defer mp.mempoolMutex.Unlock()
	mp.addTxByHashNoLock(tx)
	mp.senders.add(string(tx.sender), tx)
	mp.transactionsCount++
}

func countPromptTokensForTest(t *testing.T, prompt string) uint64 {
	t.Helper()

	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	require.NoError(t, err)

	numTokens, err := enc.Count(prompt)
	require.NoError(t, err)

	return uint64(numTokens)
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
