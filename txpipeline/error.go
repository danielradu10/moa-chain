package txpipeline

import "errors"

var (
	// ErrNilTransaction is returned when a nil transaction is submitted.
	ErrNilTransaction = errors.New("nil transaction")
	// ErrEmptyTxHash is returned when a transaction has an empty hash.
	ErrEmptyTxHash = errors.New("empty transaction hash")
	// ErrEmptyLabelResults is returned when LabelBatch returns no results for a transaction.
	ErrEmptyLabelResults = errors.New("label batch returned no results")
	// ErrEmptyAnswerResults is returned when AnswerBatch returns no results for a transaction.
	ErrEmptyAnswerResults = errors.New("answer batch returned no results")
)
