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
	// NonRelatedTransactionHashes contains the tx hashes of transactions whose
	// dominant quorum label was non_related. These transactions are excluded from
	// mini-round two answer collection and carry a NonRelatedTransaction status.
	// The slice is sorted for deterministic finalization.
	NonRelatedTransactionHashes   []string
	AggregatedExecutionResults    AggregatedExecutionResults
	// AnswerEvidence is verified in mini-round two and retained so later rounds
	// can audit the producer answers behind every classification.
	AnswerEvidence *AggregatedExecutionResultsMessage
	// AnswerClassifications is finalized after certificate verification and is
	// consumed by mini-round three when selecting canonical correct answers.
	AnswerClassifications []TransactionAnswerClassification
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

// BlockBodyExecutionResultMROne defines the execution result of a block in mini-round one.
// Contains the subdomains extracted by the validator after labeling each transaction.
// MRO is the acronym for Mini-Round One.
type BlockBodyExecutionResultMROne struct {
	Transactions     []Transaction
	TotalConsumption uint64
	Subdomains       Subdomains
}

// BlockBodyExecutionResultMRTwo defines the execution result of a block in mini-round two
type BlockBodyExecutionResultMRTwo struct {
	TxsResults       []TransactionResult
	TotalConsumption uint64
	BlockHash        []byte
}

// AggregatedExecutionResults defines the finalized mini-round two execution results.
// Results are sorted deterministically by transaction hash.
type AggregatedExecutionResults []AggregatedTransactionExecutionResults

// AggregatedTransactionExecutionResults contains all collected answers for a single transaction.
// Answers are aligned with the deterministic signer order from the verified mini-round two certificate.
type AggregatedTransactionExecutionResults struct {
	TxHash  []byte
	Answers []TransactionResult
}
