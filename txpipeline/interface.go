package txpipeline

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
