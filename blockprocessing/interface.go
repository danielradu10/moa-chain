package blockprocessing

import (
	"moa-chain/data"
)

// BlockCreator defines the interface of a block creator
type BlockCreator interface {
	ProposeBlockAndDomains() (*data.Block, data.Subdomains, []byte, error)
}

// BlockProcessor defines what a BlockProcessor should do
type BlockProcessor interface {
	ValidateBlock(block *data.Block) ([]byte, data.Subdomains, []byte, error)
	ExecuteBlockPrompts(block *data.BlockBody) (*data.BlockBodyExecutionResultMRTwo, error)
}

type LabelsValidator interface {
	ValidateLabels(labelsSubdomains data.Subdomains) error
	// AggregateLabels derives the block-level subdomain frequency map and the
	// set of transaction hashes whose dominant quorum label is non_related.
	// Non-related transactions are excluded from mini-round two.
	AggregateLabels(aggregatedSubdomains []data.Subdomains, consensusGroupSize uint64) (data.SubdomainsFrequency, []string, error)
}
