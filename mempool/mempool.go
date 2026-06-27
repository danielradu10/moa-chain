package mempool

import (
	"container/heap"

	"log/slog"
	"sync"

	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/state"

	"github.com/tiktoken-go/tokenizer"
)

const (
	maxBlockConsumption = 10000
)

// memPool is a data structure containing our transactions, grouped by sender and sorted by nonce and cached by TxHash
type memPool struct {
	transactionsCount  uint64
	transactionsByHash map[string]data.Transaction
	senders            *sendersMap

	tokensConfig map[string]uint64
	mempoolMutex sync.RWMutex
	logger       *slog.Logger
}

// NewMemPool returns a new mempool
func NewMemPool(loggers ...*slog.Logger) *memPool {
	return &memPool{
		transactionsByHash: make(map[string]data.Transaction),
		senders:            newSendersMap(),
		logger:             logging.FromOptional(loggers...),
	}
}

// AddTransaction adds a transaction into the mempool.
// Saves the transactions by its txHash and adds it to a map of senders.
// Each sender has a sorted transactions list.
func (mp *memPool) AddTransaction(transaction data.Transaction) error {
	mp.mempoolMutex.Lock()
	defer mp.mempoolMutex.Unlock()

	if transaction == nil {
		mp.logger.Error("mempool.AddTransaction rejected nil transaction")
		return ErrNilTransaction
	}

	// check if the transaction is not already added (case of duplicates)
	if mp.hasTransactionNoLock(transaction) {
		mp.logger.Info("mempool.AddTransaction ignored duplicate transaction", "txHash", string(transaction.GetTxHash()))
		return nil
	}

	err := mp.precomputeTxFields(transaction)
	if err != nil {
		return err
	}

	mp.addTxByHashNoLock(transaction)

	sender := transaction.GetSender()
	mp.senders.add(string(sender), transaction)

	mp.transactionsCount++
	mp.logger.Info(
		"mempool.AddTransaction added transaction",
		"txHash", string(transaction.GetTxHash()),
		"sender", string(sender),
		"nonce", transaction.GetNonce(),
		"estimatedConsumption", transaction.GetEstimatedConsumption(),
		"estimatedScore", transaction.GetEstimatedScore(),
		"numTransactions", mp.transactionsCount,
	)
	return nil
}

func (mp *memPool) hasTransactionNoLock(transaction data.Transaction) bool {
	txHash := transaction.GetTxHash()
	_, ok := mp.transactionsByHash[string(txHash)]
	return ok
}

// addTxByHashNoLock saves a transaction by its hash; it has to be called under a mutex protection
func (mp *memPool) addTxByHashNoLock(transaction data.Transaction) {
	txHash := transaction.GetTxHash()
	_, ok := mp.transactionsByHash[string(txHash)]
	if !ok {
		mp.transactionsByHash[string(txHash)] = transaction
	}
}

// precomputeTxFields precomputes the necessary fields of a transaction for SelectTransactions.
// (i.e. score, estimated gas consumption and other fields)
func (mp *memPool) precomputeTxFields(transaction data.Transaction) error {
	estimatedNumTokens, err := mp.estimateNumTokens(transaction)
	if err != nil {
		return err
	}

	transaction.SetEstimatedConsumption(estimatedNumTokens)

	tip := transaction.GetTip()

	score := tip / estimatedNumTokens
	transaction.SetEstimatedScore(score)
	mp.logger.Debug(
		"mempool.precomputeTxFields computed transaction selection fields",
		"txHash", string(transaction.GetTxHash()),
		"tip", tip,
		"estimatedConsumption", estimatedNumTokens,
		"estimatedScore", score,
	)

	return nil
}

// estimateNumTokens estimates the number of tokens taking into consideration numTokens from tokenizer,
// numTokens from thinking mode, numTokens from output mode
// TODO search for keywords which usually increase the number of tokens
func (mp *memPool) estimateNumTokens(transaction data.Transaction) (uint64, error) {
	numTokensFromPrompt, err := mp.calculateNumTokensFromPrompt(transaction)
	if err != nil {
		return 0, err
	}

	outputUserDimension := transaction.GetUserOutputDimension()
	thinkingMode := transaction.GetThinkingMode()

	thinkingEstimatedTokens := mp.tokensConfig[thinkingMode]
	outputUserDimensionEstimatedTokens := mp.tokensConfig[outputUserDimension]

	estimatedTokens := numTokensFromPrompt + thinkingEstimatedTokens + outputUserDimensionEstimatedTokens
	return estimatedTokens, nil
}

// calculateNumTokensFromPrompt tokenizes the prompt to get the number of tokens
func (mp *memPool) calculateNumTokensFromPrompt(transaction data.Transaction) (uint64, error) {
	prompt := transaction.GetPrompt()

	enc, err := tokenizer.Get(tokenizer.Cl100kBase)
	if err != nil {
		return 0, err
	}

	numTokens, err := enc.Count(string(prompt))
	if err != nil {
		return 0, err
	}

	return uint64(numTokens), nil
}

// snapshot does a snapshot of each important map from the MemPool.
// The method is used in the SelectTransactions so that the selection can be made without concurrent interferences.
func (mp *memPool) snapshot() (map[string]data.Transaction, *sendersMap) {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	transactionsByHashSnapshot := make(map[string]data.Transaction, len(mp.transactionsByHash))
	for txHash, tx := range mp.transactionsByHash {
		transactionsByHashSnapshot[txHash] = tx
	}

	sendersMapSnapshot := mp.senders.snapshot()
	mp.logger.Debug(
		"mempool.snapshot created selection snapshot",
		"numTransactions", len(transactionsByHashSnapshot),
		"numSenders", sendersMapSnapshot.numAddresses(),
	)
	return transactionsByHashSnapshot, sendersMapSnapshot
}

// SelectTransactions selects transactions from the pool, using the current accounts state on chain and the given transactions comparator
func (mp *memPool) SelectTransactions(
	accountsState state.AccountsState,
) []data.Transaction {
	_, sendersMapSnapshot := mp.snapshot()
	mp.logger.Info("mempool.SelectTransactions started", "numSenders", sendersMapSnapshot.numAddresses())

	txHeap, err := newTransactionsHeap(sendersMapSnapshot.numAddresses(), isTransactionMoreValuable)
	if err != nil {
		mp.logger.Error("mempool.SelectTransactions failed to create heap", "error", err)
		return nil
	}

	heap.Init(txHeap)
	for _, senderTxList := range sendersMapSnapshot.senders {
		heap.Push(txHeap, newTxHeapItem(senderTxList))
	}

	accumulatedConsumption := uint64(0)
	selectedTransactions := make([]data.Transaction, 0)
	selSession := newSelectionSession(accountsState)

	for txHeap.Len() > 0 {
		currentBestItem := heap.Pop(txHeap).(*txHeapItem)

		currentBestTransaction := currentBestItem.getCurrentTransaction()
		estimatedConsumption := currentBestTransaction.GetEstimatedConsumption()
		if accumulatedConsumption+estimatedConsumption > maxBlockConsumption {
			mp.logger.Info(
				"mempool.SelectTransactions stopped at block consumption limit",
				"txHash", string(currentBestTransaction.GetTxHash()),
				"accumulatedConsumption", accumulatedConsumption,
				"nextEstimatedConsumption", estimatedConsumption,
				"maxBlockConsumption", maxBlockConsumption,
			)
			break
		}

		if selSession.senderShouldBeSkipped(currentBestTransaction) {
			mp.logger.Debug(
				"mempool.SelectTransactions skipped sender",
				"sender", string(currentBestTransaction.GetSender()),
				"txHash", string(currentBestTransaction.GetTxHash()),
				"nonce", currentBestTransaction.GetNonce(),
			)
			continue
		}

		if !selSession.transactionShouldBeSkipped(currentBestTransaction) {
			err := selSession.OnSelectedTransaction(currentBestTransaction)
			if err != nil {
				mp.logger.Error(
					"mempool.SelectTransactions failed to update virtual state",
					"sender", string(currentBestTransaction.GetSender()),
					"txHash", string(currentBestTransaction.GetTxHash()),
					"error", err,
				)
				continue
			}

			selectedTransactions = append(selectedTransactions, currentBestTransaction)
			accumulatedConsumption += estimatedConsumption
			mp.logger.Info(
				"mempool.SelectTransactions selected transaction",
				"txHash", string(currentBestTransaction.GetTxHash()),
				"sender", string(currentBestTransaction.GetSender()),
				"nonce", currentBestTransaction.GetNonce(),
				"estimatedConsumption", estimatedConsumption,
				"estimatedScore", currentBestTransaction.GetEstimatedScore(),
				"accumulatedConsumption", accumulatedConsumption,
			)
		} else {
			mp.logger.Debug(
				"mempool.SelectTransactions skipped transaction",
				"txHash", string(currentBestTransaction.GetTxHash()),
				"sender", string(currentBestTransaction.GetSender()),
				"nonce", currentBestTransaction.GetNonce(),
			)
		}

		if currentBestItem.nextTransactionOfSenderExists() {
			currentBestItem.goToNextTransactionOfSender()
			heap.Push(txHeap, currentBestItem)
		}
	}

	mp.logger.Info(
		"mempool.SelectTransactions finished",
		"numSelectedTransactions", len(selectedTransactions),
		"accumulatedConsumption", accumulatedConsumption,
	)
	return selectedTransactions
}

// NumTransactions returns the number of transactions from the pool
func (mp *memPool) NumTransactions() uint64 {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	return uint64(len(mp.transactionsByHash))
}

// NumAddresses returns the number of addresses from the pool
func (mp *memPool) NumAddresses() uint64 {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	return mp.senders.numAddresses()
}

func (mp *memPool) getTransactionsListBySender(sender []byte) (*txList, error) {
	mp.mempoolMutex.RLock()
	defer mp.mempoolMutex.RUnlock()

	if sender == nil {
		return nil, ErrNilSender
	}

	return mp.senders.getTransactionsListBySender(sender)
}
