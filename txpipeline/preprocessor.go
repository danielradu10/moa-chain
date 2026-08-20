package txpipeline

import (
	"log/slog"
	"sync"

	"moa-chain/agent"
	"moa-chain/data"
	"moa-chain/logging"
	"moa-chain/mempool"
)

const preprocessorQueueSize = 128

// TxPreprocessorArgs holds the dependencies for NewTxPreprocessor.
type TxPreprocessorArgs struct {
	Agent   agent.BatchAgent
	Store   PrecomputedStore
	Mempool mempool.Mempool
	Logger  *slog.Logger
}

type txPreprocessor struct {
	agent   agent.BatchAgent
	store   PrecomputedStore
	mempool mempool.Mempool
	queue   chan data.Transaction
	stopCh  chan struct{}
	wg      sync.WaitGroup
	logger  *slog.Logger
}

// NewTxPreprocessor creates a TxPreprocessor. Call Start before enqueuing transactions.
func NewTxPreprocessor(args TxPreprocessorArgs) TxPreprocessor {
	return &txPreprocessor{
		agent:   args.Agent,
		store:   args.Store,
		mempool: args.Mempool,
		queue:   make(chan data.Transaction, preprocessorQueueSize),
		stopCh:  make(chan struct{}),
		logger:  logging.FromOptional(args.Logger),
	}
}

func (p *txPreprocessor) Enqueue(tx data.Transaction) {
	p.queue <- tx
}

func (p *txPreprocessor) Start() {
	p.wg.Add(1)
	go p.run()
}

// Stop signals the processing loop to exit and waits for all in-flight
// label+answer goroutines to finish before returning.
func (p *txPreprocessor) Stop() {
	close(p.stopCh)
	p.wg.Wait()
}

func (p *txPreprocessor) run() {
	defer p.wg.Done()

	for {
		select {
		case tx := <-p.queue:
			p.wg.Add(1)

			go func(t data.Transaction) {
				defer p.wg.Done()
				p.process(t)
			}(tx)

		case <-p.stopCh:
			for {
				select {
				case tx := <-p.queue:
					p.wg.Add(1)
					go func(t data.Transaction) {
						defer p.wg.Done()
						p.process(t)
					}(tx)
				default:
					return
				}
			}
		}
	}
}

func (p *txPreprocessor) process(tx data.Transaction) {
	txHash := tx.GetTxHash()

	labels, answer, err := p.labelAndAnswer(tx)
	if err != nil {
		p.logger.Error("txPreprocessor: preprocessing failed", "txHash", string(txHash), "error", err)
		return
	}

	p.store.StoreLabels(txHash, labels)
	p.store.StoreAnswer(txHash, answer)

	if err := p.mempool.AddTransaction(tx); err != nil {
		p.logger.Error("txPreprocessor: failed to add tx to mempool", "txHash", string(txHash), "error", err)
	}
}

// labelAndAnswer runs fetchLabels and fetchAnswer concurrently and returns
// both results. The first error encountered is returned.
func (p *txPreprocessor) labelAndAnswer(tx data.Transaction) ([]string, string, error) {
	type labelResult struct {
		labels []string
		err    error
	}
	type answerResult struct {
		answer string
		err    error
	}

	labelCh := make(chan labelResult, 1)
	answerCh := make(chan answerResult, 1)

	go func() { labels, err := p.fetchLabels(tx); labelCh <- labelResult{labels, err} }()
	go func() { answer, err := p.fetchAnswer(tx); answerCh <- answerResult{answer, err} }()

	lr := <-labelCh
	ar := <-answerCh

	if lr.err != nil {
		return nil, "", lr.err
	}
	if ar.err != nil {
		return nil, "", ar.err
	}

	return lr.labels, ar.answer, nil
}

func (p *txPreprocessor) fetchLabels(tx data.Transaction) ([]string, error) {
	results, err := p.agent.LabelBatch([]data.Transaction{tx})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrEmptyLabelResults
	}
	return results[0].Labels, nil
}

func (p *txPreprocessor) fetchAnswer(tx data.Transaction) (string, error) {
	results, err := p.agent.AnswerBatch([]data.Transaction{tx})
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", ErrEmptyAnswerResults
	}
	return results[0].Answer, nil
}
