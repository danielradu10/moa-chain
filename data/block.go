package data

// Block defines a block that needs to be validated
type Block struct {
	Header BlockHeader
	Body   BlockBody
}

// BlockHeader defines the header of a block
type BlockHeader struct {
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
