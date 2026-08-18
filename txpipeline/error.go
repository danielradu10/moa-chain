package txpipeline

import "errors"

var (
	// ErrEmptyLabelResults is returned when LabelBatch returns no results for a transaction.
	ErrEmptyLabelResults = errors.New("label batch returned no results")
	// ErrEmptyAnswerResults is returned when AnswerBatch returns no results for a transaction.
	ErrEmptyAnswerResults = errors.New("answer batch returned no results")
)
