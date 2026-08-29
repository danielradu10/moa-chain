package txpipeline

import (
	"log/slog"
	"sync"

	"moa-chain/broadcast"
	"moa-chain/data"
	"moa-chain/logging"
)

// TxInterceptorArgs holds the dependencies for NewTxInterceptor.
type TxInterceptorArgs struct {
	// Inbox is the channel on which peer nodes deliver raw transactions.
	// Register this channel with the TxPeerRegistry so the broadcaster can
	// reach this node.
	Inbox        chan data.Transaction
	Preprocessor TxPreprocessor
	Broadcaster  broadcast.TxBroadcaster
	SelfID       string
	Logger       *slog.Logger
	// OnTxSubmitted is an optional hook called when a transaction passes
	// validation and deduplication. Wire TxTracker.OnSubmitted here.
	OnTxSubmitted func(data.Transaction)
}

type txInterceptor struct {
	inbox         chan data.Transaction
	preprocessor  TxPreprocessor
	broadcaster   broadcast.TxBroadcaster
	selfID        string
	onTxSubmitted func(data.Transaction)

	mu     sync.Mutex
	seen   map[string]struct{}
	stopCh chan struct{}
	wg     sync.WaitGroup
	logger *slog.Logger
}

// NewTxInterceptor creates a TxInterceptor. Call Start before transactions arrive.
func NewTxInterceptor(args TxInterceptorArgs) TxInterceptor {
	return &txInterceptor{
		inbox:         args.Inbox,
		preprocessor:  args.Preprocessor,
		broadcaster:   args.Broadcaster,
		selfID:        args.SelfID,
		onTxSubmitted: args.OnTxSubmitted,
		seen:          make(map[string]struct{}),
		stopCh:        make(chan struct{}),
		logger:        logging.FromOptional(args.Logger),
	}
}

// Submit validates and deduplicates the transaction, enqueues it for
// preprocessing, and broadcasts the raw transaction to all peers.
func (i *txInterceptor) Submit(tx data.Transaction) error {
	if err := i.validate(tx); err != nil {
		return err
	}
	if !i.markSeen(tx.GetTxHash()) {
		i.logger.Debug("txInterceptor: duplicate transaction ignored on Submit",
			"txHash", string(tx.GetTxHash()))
		return nil
	}

	if i.onTxSubmitted != nil {
		i.onTxSubmitted(tx)
	}
	i.broadcaster.BroadcastTransaction(tx, i.selfID)
	i.preprocessor.Enqueue(tx)
	return nil
}

func (i *txInterceptor) Start() {
	i.wg.Add(1)
	go i.run()
}

func (i *txInterceptor) Stop() {
	close(i.stopCh)
	i.wg.Wait()
}

// run drains the peer inbox. Transactions arriving here were already broadcast
// by another node, so they are only enqueued for local preprocessing.
func (i *txInterceptor) run() {
	defer i.wg.Done()

	for {
		select {
		case tx := <-i.inbox:
			if err := i.validate(tx); err != nil {
				i.logger.Error("txInterceptor: invalid transaction received from peer",
					"error", err)
				continue
			}
			if !i.markSeen(tx.GetTxHash()) {
				continue
			}
			if i.onTxSubmitted != nil {
				i.onTxSubmitted(tx)
			}
			i.preprocessor.Enqueue(tx)

		case <-i.stopCh:
			return
		}
	}
}

func (i *txInterceptor) validate(tx data.Transaction) error {
	if tx == nil {
		return ErrNilTransaction
	}
	if len(tx.GetTxHash()) == 0 {
		return ErrEmptyTxHash
	}
	return nil
}

// markSeen records txHash as seen and returns true if this is the first time.
func (i *txInterceptor) markSeen(txHash []byte) bool {
	i.mu.Lock()
	defer i.mu.Unlock()

	key := string(txHash)
	if _, exists := i.seen[key]; exists {
		return false
	}
	i.seen[key] = struct{}{}
	return true
}
