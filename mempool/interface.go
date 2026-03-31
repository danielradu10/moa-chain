package mempool

// Transaction defines what data a transaction should return
type Transaction interface {
	GetNonce() uint64
	GetPrompt() []byte
	GetSender() []byte
	GetReceiver() []byte
	GetTransferredValue() uint64
	GetTip() uint64
	GetTimestamp() uint64
	GetTxHash() []byte

	GetDomainLabel() []byte
	GetNumInputTokens() uint64
	GetNumEstimatedOutputTokens() uint64
	GetEstimatedConsumption() uint64
	GetEstimatedFee() uint64
	GetEstimatedScore() uint64

	IsInterfaceNil() bool
}
