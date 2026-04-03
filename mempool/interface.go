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
	GetUserOutputDimension() string
	GetThinkingMode() string
	GetEstimatedConsumption() uint64
	GetEstimatedFee() uint64
	GetEstimatedScore() uint64

	SetNonce(nonce uint64)
	SetPrompt(prompt []byte)
	SetSender(sender []byte)
	SetReceiver(receiver []byte)
	SetTransferredValue(value uint64)
	SetTip(tip uint64)
	SetTimestamp(timestamp uint64)
	SetTxHash(txHash []byte)

	SetDomainLabel(domainLabel []byte)
	SetNumInputTokens(numInputTokens uint64)
	SetUserOutputDimension(userOutputDimension string)
	SetThinkingMode(thinkingMode string)
	SetEstimatedConsumption(estimatedConsumption uint64)
	SetEstimatedFee(estimatedFee uint64)
	SetEstimatedScore(estimatedScore uint64)

	IsInterfaceNil() bool
}
