package signing

// MessageSigner describes what a MessageSigner should do
type MessageSigner interface {
	ID() string
	Sign(message []byte) ([]byte, error)
	Verify(publicKey []byte, message []byte, signature []byte) error
}
