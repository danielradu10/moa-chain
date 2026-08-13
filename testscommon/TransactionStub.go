package testscommon

import "moa-chain/data"

type TransactionStub struct {
	nonce                uint64
	prompt               []byte
	sender               []byte
	receiver             []byte
	transferredValue     uint64
	tip                  uint64
	timestamp            uint64
	txHash               []byte
	domainLabels         []string
	numInputTokens       uint64
	userOutputDimension  string
	thinkingMode         string
	estimatedConsumption uint64
	estimatedFee         uint64
	estimatedScore       uint64
}

func (tx *TransactionStub) SetNonce(nonce uint64) {
	tx.nonce = nonce
}

func (tx *TransactionStub) SetPrompt(prompt []byte) {
	tx.prompt = prompt
}

func (tx *TransactionStub) SetSender(sender []byte) {
	tx.sender = sender
}

func (tx *TransactionStub) SetReceiver(receiver []byte) {
	tx.receiver = receiver
}

func (tx *TransactionStub) SetTransferredValue(value uint64) {
	tx.transferredValue = value
}

func (tx *TransactionStub) SetTip(tip uint64) {
	tx.tip = tip
}

func (tx *TransactionStub) SetTimestamp(timestamp uint64) {
	tx.timestamp = timestamp
}

func (tx *TransactionStub) SetTxHash(txHash []byte) {
	tx.txHash = txHash
}

func (tx *TransactionStub) SetDomainLabels(domainLabels []string) {
	tx.domainLabels = domainLabels
}

func (tx *TransactionStub) SetNumInputTokens(numInputTokens uint64) {
	tx.numInputTokens = numInputTokens
}

func (tx *TransactionStub) SetUserOutputDimension(userOutputDimension string) {
	tx.userOutputDimension = userOutputDimension
}

func (tx *TransactionStub) SetThinkingMode(thinkingMode string) {
	tx.thinkingMode = thinkingMode
}

func (tx *TransactionStub) SetEstimatedConsumption(estimatedConsumption uint64) {
	tx.estimatedConsumption = estimatedConsumption
}

func (tx *TransactionStub) SetEstimatedFee(estimatedFee uint64) {
	tx.estimatedFee = estimatedFee
}

func (tx *TransactionStub) SetEstimatedScore(estimatedScore uint64) {
	tx.estimatedScore = estimatedScore
}

func (tx *TransactionStub) GetNonce() uint64               { return tx.nonce }
func (tx *TransactionStub) GetPrompt() []byte              { return tx.prompt }
func (tx *TransactionStub) GetSender() []byte              { return tx.sender }
func (tx *TransactionStub) GetReceiver() []byte            { return tx.receiver }
func (tx *TransactionStub) GetTransferredValue() uint64    { return tx.transferredValue }
func (tx *TransactionStub) GetTip() uint64                 { return tx.tip }
func (tx *TransactionStub) GetTimestamp() uint64           { return tx.timestamp }
func (tx *TransactionStub) GetTxHash() []byte              { return tx.txHash }
func (tx *TransactionStub) GetDomainLabels() []string      { return tx.domainLabels }
func (tx *TransactionStub) GetNumInputTokens() uint64      { return tx.numInputTokens }
func (tx *TransactionStub) GetUserOutputDimension() string { return tx.userOutputDimension }
func (tx *TransactionStub) GetThinkingMode() string        { return tx.thinkingMode }
func (tx *TransactionStub) GetEstimatedConsumption() uint64 {
	return tx.estimatedConsumption
}
func (tx *TransactionStub) GetEstimatedFee() uint64   { return tx.estimatedFee }
func (tx *TransactionStub) GetEstimatedScore() uint64 { return tx.estimatedScore }
func (tx *TransactionStub) IsInterfaceNil() bool { return tx == nil }

func (tx *TransactionStub) Clone() data.Transaction {
	cloned := &TransactionStub{
		nonce:                tx.nonce,
		prompt:               append([]byte(nil), tx.prompt...),
		sender:               append([]byte(nil), tx.sender...),
		receiver:             append([]byte(nil), tx.receiver...),
		transferredValue:     tx.transferredValue,
		tip:                  tx.tip,
		timestamp:            tx.timestamp,
		txHash:               append([]byte(nil), tx.txHash...),
		numInputTokens:       tx.numInputTokens,
		userOutputDimension:  tx.userOutputDimension,
		thinkingMode:         tx.thinkingMode,
		estimatedConsumption: tx.estimatedConsumption,
		estimatedFee:         tx.estimatedFee,
		estimatedScore:       tx.estimatedScore,
	}
	if tx.domainLabels != nil {
		cloned.domainLabels = append([]string(nil), tx.domainLabels...)
	}
	return cloned
}
