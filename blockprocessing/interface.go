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
}

type LabelsValidator interface {
	ValidateLabels(labelsSubdomains data.Subdomains) error
	AggregateLabels(aggregatedSubdomains []data.Subdomains, consensusGroupSize uint64) (data.SubdomainsFrequency, error)
}
