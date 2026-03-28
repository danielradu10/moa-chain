package mempool

func createTx(nonce uint64, estimatedConsumption uint64, txHash []byte) *transaction {
	return &transaction{
		nonce:                nonce,
		estimatedConsumption: estimatedConsumption,
		txHash:               txHash,
	}
}
