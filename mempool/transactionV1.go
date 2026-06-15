package mempool

// transaction is the data structure used to define a transaction
type transaction struct {
	// existent fields
	nonce            uint64
	prompt           []byte
	sender           []byte
	receiver         []byte // in case we have a reward transaction
	transferredValue uint64 // this will only be used for reward transactions
	tip              uint64 // this is a field set by the user in order to promote its own transaction
	timestamp        uint64
	txHash           []byte

	// to be computed fields
	domainLabel          []byte
	numInputTokens       uint64
	userOutputDimension  string
	thinkingMode         string // we can have four basic thinking modes:fast, standard, thinking, deep research.
	estimatedConsumption uint64 // we will estimate the consumption by taking into consideration the fields from above.
	estimatedFee         uint64
	estimatedScore       uint64
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

func (tx *transaction) GetUserOutputDimension() string {
	return tx.userOutputDimension
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

func (tx *transaction) GetDomainLabel() []byte {
	return tx.domainLabel
}

func (tx *transaction) GetThinkingMode() string {
	return tx.thinkingMode
}

func (tx *transaction) SetNonce(nonce uint64) {
	tx.nonce = nonce
}

func (tx *transaction) SetPrompt(prompt []byte) {
	tx.prompt = prompt
}

func (tx *transaction) SetSender(sender []byte) {
	tx.sender = sender
}

func (tx *transaction) SetReceiver(receiver []byte) {
	tx.receiver = receiver
}

func (tx *transaction) SetTransferredValue(value uint64) {
	tx.transferredValue = value
}

func (tx *transaction) SetTip(tip uint64) {
	tx.tip = tip
}

func (tx *transaction) SetTimestamp(timestamp uint64) {
	tx.timestamp = timestamp
}

func (tx *transaction) SetTxHash(txHash []byte) {
	tx.txHash = txHash
}

func (tx *transaction) SetDomainLabel(domainLabel []byte) {
	tx.domainLabel = domainLabel
}

func (tx *transaction) SetNumInputTokens(numInputTokens uint64) {
	tx.numInputTokens = numInputTokens
}

func (tx *transaction) SetUserOutputDimension(userOutputDimension string) {
	tx.userOutputDimension = userOutputDimension
}

func (tx *transaction) SetThinkingMode(thinkingMode string) {
	tx.thinkingMode = thinkingMode
}

func (tx *transaction) SetEstimatedConsumption(estimatedConsumption uint64) {
	tx.estimatedConsumption = estimatedConsumption
}

func (tx *transaction) SetEstimatedFee(estimatedFee uint64) {
	tx.estimatedFee = estimatedFee
}

func (tx *transaction) SetEstimatedScore(estimatedScore uint64) {
	tx.estimatedScore = estimatedScore
}
