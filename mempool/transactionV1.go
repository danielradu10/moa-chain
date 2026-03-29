package mempool

// transaction is the data structure used to define a transaction
type transaction struct {
	// existent fields
	nonce            uint64
	prompt           []byte
	sender           []byte
	receiver         []byte // in case we have a reward transaction
	transferredValue uint64
	tip              uint64
	timestamp        uint64
	txHash           []byte

	// to be computed fields
	numInputTokens           uint64
	numEstimatedOutputTokens uint64
	estimatedConsumption     uint64
	estimatedFee             uint64
	estimatedScore           uint64
}

func (tx *transaction) GetNonce() uint64 {
	return tx.nonce
}

func (tx *transaction) GetPrompt() []byte {
	return tx.prompt
}

func (tx *transaction) GetSender() []byte {
	return tx.sender
}

func (tx *transaction) GetReceiver() []byte {
	return tx.receiver
}

func (tx *transaction) GetTransferredValue() uint64 {
	return tx.transferredValue
}

func (tx *transaction) GetTip() uint64 {
	return tx.tip
}

func (tx *transaction) GetTimestamp() uint64 {
	return tx.timestamp
}

func (tx *transaction) GetTxHash() []byte {
	return tx.txHash
}

func (tx *transaction) GetNumInputTokens() uint64 {
	return tx.numInputTokens
}

func (tx *transaction) GetNumEstimatedOutputTokens() uint64 {
	return tx.numEstimatedOutputTokens
}

func (tx *transaction) GetEstimatedFee() uint64 {
	return tx.estimatedFee
}

func (tx *transaction) GetEstimatedScore() uint64 {
	return tx.estimatedScore
}

func (tx *transaction) GetEstimatedConsumption() uint64 {
	return tx.estimatedConsumption
}

func (tx *transaction) IsInterfaceNil() bool {
	return tx == nil
}
