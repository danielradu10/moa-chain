package explorer

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status           string `json:"status"`
	ChainLength      uint64 `json:"chain_length"`
	CurrentRound     uint64 `json:"current_round"`
	CurrentMiniRound uint64 `json:"current_mini_round"`
	CurrentEpoch     uint64 `json:"current_epoch"`
}

// ErrorResponse is the standard error envelope for all 4xx/5xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}

// BlockResponse is returned by GET /api/v1/blocks/{hash}.
// Represents a fully finalized chain block with all mini-round data.
type BlockResponse struct {
	HeaderHash            string                    `json:"header_hash"`
	PreviousHash          string                    `json:"previous_hash"`
	Round                 uint64                    `json:"round"`
	Epoch                 uint64                    `json:"epoch"`
	Transactions          []TxSummary               `json:"transactions"`
	SubdomainsFrequency   map[string]uint64         `json:"subdomains_frequency,omitempty"`
	NonRelatedTxHashes    []string                  `json:"non_related_tx_hashes,omitempty"`
	AnswerClassifications []TxClassificationSummary `json:"answer_classifications,omitempty"`
	FinalAnswers          []FinalAnswerSummary      `json:"final_answers,omitempty"`
}

// TxSummary is the compact transaction representation used in block and round responses.
type TxSummary struct {
	TxHash string   `json:"tx_hash"`
	Sender string   `json:"sender"`
	Prompt string   `json:"prompt"`
	Labels []string `json:"labels,omitempty"`
}

// TxClassificationSummary summarises the MR2 classification outcome for one transaction.
type TxClassificationSummary struct {
	TxHash       string `json:"tx_hash"`
	Status       string `json:"status"`
	CorrectCount uint64 `json:"correct_count"`
}

// FinalAnswerSummary summarises the MR3 synthesis outcome for one transaction.
type FinalAnswerSummary struct {
	TxHash string `json:"tx_hash"`
	Status string `json:"status"`
	Answer string `json:"answer,omitempty"`
}

// RoundSummary is the compact round representation used in the rounds list.
type RoundSummary struct {
	Round   uint64 `json:"round"`
	Epoch   uint64 `json:"epoch"`
	Status  string `json:"status"`
	TxCount int    `json:"tx_count"`
	BlockHash string `json:"block_hash,omitempty"`
}

// RoundResponse is returned by GET /api/v1/rounds/{round}.
// Mini-round fields are omitted when that mini-round has not yet completed.
type RoundResponse struct {
	Round  uint64       `json:"round"`
	Epoch  uint64       `json:"epoch"`
	Status string       `json:"status"`
	MR1    *MR1Response `json:"mr1,omitempty"`
	MR2    *MR2Response `json:"mr2,omitempty"`
	MR3    *MR3Response `json:"mr3,omitempty"`
}

// MR1Response holds the mini-round-one view of a round.
type MR1Response struct {
	BlockHash           string            `json:"block_hash"`
	Transactions        []TxSummary       `json:"transactions"`
	SubdomainsFrequency map[string]uint64 `json:"subdomains_frequency"`
	NonRelatedTxHashes  []string          `json:"non_related_tx_hashes,omitempty"`
}

// MR2Response holds the mini-round-two view of a round.
type MR2Response struct {
	BlockHash             string                    `json:"block_hash"`
	AnswerClassifications []TxClassificationSummary `json:"answer_classifications"`
}

// MR3Response holds the mini-round-three view of a round.
type MR3Response struct {
	BlockHash    string               `json:"block_hash"`
	FinalAnswers []FinalAnswerSummary `json:"final_answers"`
}

// LiveRoundResponse is returned by GET /api/v1/live/round.
// It reflects the finest-grained step the local node is currently executing.
type LiveRoundResponse struct {
	Epoch     uint64 `json:"epoch"`
	Round     uint64 `json:"round"`
	MiniRound uint64 `json:"mini_round"`
	Step      string `json:"step"`
}

// SubmitTransactionRequest is the body of POST /api/v1/transactions.
type SubmitTransactionRequest struct {
	Sender string `json:"sender"`
	Prompt string `json:"prompt"`
	Nonce  uint64 `json:"nonce"`
	Tip    uint64 `json:"tip"`
}

// SubmitTransactionResponse is returned by POST /api/v1/transactions.
type SubmitTransactionResponse struct {
	TxHash    string `json:"tx_hash"`
	Timestamp uint64 `json:"timestamp"`
}

// ValidatorLabelVote records the domain labels one MR1 validator assigned to a
// specific transaction.
type ValidatorLabelVote struct {
	ValidatorID string   `json:"validator_id"`
	Labels      []string `json:"labels"`
}

// JudgeVerdict records one judge's category decision for a specific candidate answer.
type JudgeVerdict struct {
	JudgeID  string `json:"judge_id"`
	Category string `json:"category"` // CORRECT | HALLUCINATION | MALICIOUS | WRONG
}

// ValidatorAnswer holds one MR2 validator's individual answer, the consensus
// classification, the per-category judge vote counts, and the full per-judge verdicts.
type ValidatorAnswer struct {
	ValidatorID        string         `json:"validator_id"`
	Answer             string         `json:"answer"`
	Category           string         `json:"category"`              // consensus outcome
	Consumption        uint64         `json:"consumption,omitempty"` // token consumption
	CorrectVotes       uint64         `json:"correct_votes"`
	HallucinationVotes uint64         `json:"hallucination_votes"`
	MaliciousVotes     uint64         `json:"malicious_votes"`
	WrongVotes         uint64         `json:"wrong_votes"`
	JudgeVerdicts      []JudgeVerdict `json:"judge_verdicts,omitempty"`
}

// TransactionResponse is returned by GET /api/v1/transactions/{hash}.
// Fields are populated progressively: basic status is always present;
// sender/prompt/labels appear once preprocessing starts; block_hash,
// final_answer, and final_status appear only when finalized.
// ValidatorAnswers and Votes are populated only for finalized transactions
// that passed through MR2.
type TransactionResponse struct {
	TxHash           string             `json:"tx_hash"`
	Status           string             `json:"status"`
	Round            uint64             `json:"round,omitempty"`
	Sender           string             `json:"sender,omitempty"`
	Prompt           string             `json:"prompt,omitempty"`
	Labels           []string           `json:"labels,omitempty"`
	LocalAnswer      string             `json:"local_answer,omitempty"`
	BlockHash        string             `json:"block_hash,omitempty"`
	FinalAnswer      string             `json:"final_answer,omitempty"`
	FinalStatus      string             `json:"final_status,omitempty"`
	ValidatorAnswers    []ValidatorAnswer    `json:"validator_answers,omitempty"`
	LabelVotes         []ValidatorLabelVote `json:"label_votes,omitempty"`
	SynthesisProposer  string               `json:"synthesis_proposer,omitempty"`
	SynthesisApprovers []string             `json:"synthesis_approvers,omitempty"`
}
