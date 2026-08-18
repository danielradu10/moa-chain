package txpipeline

import "moa-chain/data"

// TxPreprocessor runs labeling and answering concurrently for each incoming
// transaction, stores the results in a PrecomputedStore, and only then makes
// the transaction eligible for inclusion in the mempool.
type TxPreprocessor interface {
	// Enqueue submits a transaction for asynchronous preprocessing.
	Enqueue(tx data.Transaction)
	// Start launches the background processing goroutine.
	Start()
	// Stop signals shutdown and blocks until all in-flight work is done.
	Stop()
}

// PrecomputedStore holds node-local labels and answers keyed by transaction
// hash. Entries are written by the TxPreprocessor after the model calls
// complete and read by the MR1/MR2 body executors during consensus. All
// methods are safe for concurrent use.
type PrecomputedStore interface {
	StoreLabels(txHash []byte, labels []string)
	GetLabels(txHash []byte) ([]string, bool)
	StoreAnswer(txHash []byte, answer string)
	GetAnswer(txHash []byte) (string, bool)
	// Remove deletes all precomputed data for txHash. Called after the
	// transaction is finalized and appended to the chain.
	Remove(txHash []byte)
}
