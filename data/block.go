package data

// Block defines a block that needs to be validated.
type Block struct {
	Header BlockHeader
	Body   BlockBody
}

// BlockOnChain defines the block that is finalized on chain.
type BlockOnChain struct {
	Block                 Block
	SubdomainsFrequencies SubdomainsFrequency
}

// BlockHeader defines the header of a block
type BlockHeader struct {
	BodyHash []byte

	HeaderHash   []byte
	PreviousHash []byte

	RootHash         []byte
	PreviousRootHash []byte

	Nonce     uint64
	Round     uint64
	MiniRound uint64
	Epoch     uint64
}

// BlockBody defines the body of a block
type BlockBody struct {
	Transactions []Transaction
}

// Subdomains defines the label extracted by a validator for each transaction.
type Subdomains map[string][]string

// SubdomainsFrequency defines the frequency of the labels extracted in a proposed block.
type SubdomainsFrequency map[string]uint64

// BlockBodyExecutionResult defines the execution result of a block in mini-round one.
// Contains the subdomains extracted by the validator after labeling each transaction.
type BlockBodyExecutionResult struct {
	Transactions     []Transaction
	TotalConsumption uint64
	Subdomains       Subdomains
}
