package data

// Block defines a block that needs to be validated.
type Block struct {
	Header BlockHeader
	Body   BlockBody
}

// BlockOnChain defines the block that is finalized on chain.
// It carries the canonical output of a full round (MR1 → MR2 → MR3).
type BlockOnChain struct {
	Header                ChainBlockHeader
	Body                  BlockBody
	SubdomainsFrequencies SubdomainsFrequency
	// NonRelatedTransactionHashes contains the tx hashes of transactions whose
	// dominant quorum label was non_related. These transactions are excluded from
	// mini-round two answer collection and carry a NonRelatedTransaction status.
	// The slice is sorted for deterministic finalization.
	NonRelatedTransactionHashes []string
	AggregatedExecutionResults  AggregatedExecutionResults
	// AnswerEvidence is verified in mini-round two and retained so later rounds
	// can audit the producer answers behind every classification.
	AnswerEvidence *AggregatedExecutionResultsMessage
	// AnswerClassifications is finalized after certificate verification and is
	// consumed by mini-round three when selecting canonical correct answers.
	AnswerClassifications []TransactionAnswerClassification
	// ClassificationVotes contains the individual signed judge votes from MR2.
	// Retained for auditability: each vote records which judge assigned which
	// category to each candidate answer. Empty for blocks with no transactions.
	ClassificationVotes []AnswerClassificationVote
	// LabelVotes contains each MR1 validator's individual per-transaction label
	// assignments, retained for auditability. Aligned with SubdomainsFrequencies.
	LabelVotes []ValidatorLabelVote

	// SynthesisProposerID is the MR3 leader who proposed the synthesized answer.
	SynthesisProposerID string
	// SynthesisApprovers lists the validator IDs who voted to approve the synthesis.
	// Validators who rejected silently abstained and are not listed.
	SynthesisApprovers []string

	// FinalAnswers is populated by mini-round three. Each entry holds the
	// synthesized answer for eligible transactions or a SKIPPED status for
	// transactions that did not reach a correct-answer quorum in MR2.
	// The slice is sorted by transaction hash.
	FinalAnswers []FinalAnswer
}

// BlockHeader defines the header of a mini-round block used during consensus.
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

// ChainBlockHeader defines the header of a block stored on the canonical chain.
// It is the header of the full-round canonical output, distinct from the
// per-mini-round BlockHeader used during consensus.
// MiniRound is intentionally absent: the chain block is always the output of the
// complete round (all three mini-rounds), so tracking the mini-round here is redundant.
type ChainBlockHeader struct {
	BodyHash []byte

	HeaderHash   []byte
	PreviousHash []byte

	RootHash         []byte
	PreviousRootHash []byte

	Nonce uint64
	Round uint64
	Epoch uint64
}

// ChainBlockHeaderFromMiniRound copies a mini-round BlockHeader into a ChainBlockHeader.
// Used at MR1 finalization when the canonical block is first assembled.
func ChainBlockHeaderFromMiniRound(h BlockHeader) ChainBlockHeader {
	return ChainBlockHeader{
		BodyHash:         h.BodyHash,
		HeaderHash:       h.HeaderHash,
		PreviousHash:     h.PreviousHash,
		RootHash:         h.RootHash,
		PreviousRootHash: h.PreviousRootHash,
		Nonce:            h.Nonce,
		Round:            h.Round,
		Epoch:            h.Epoch,
	}
}

// BlockBody defines the body of a block
type BlockBody struct {
	Transactions []Transaction
}

// Subdomains defines the label extracted by a validator for each transaction.
type Subdomains map[string][]string

// ValidatorLabelVote records the per-transaction domain labels assigned by one
// validator during MR1. Labels is keyed by raw tx-hash bytes cast to string,
// matching the Subdomains map convention used throughout MR1.
type ValidatorLabelVote struct {
	ValidatorID string
	Labels      Subdomains
}

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
