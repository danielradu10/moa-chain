package signing

import (
	"crypto/ed25519"
	_ "crypto/ed25519"
	_ "crypto/rand"
)

// signer implements the MessageSigner interface
type signer struct {
	id         string
	privateKey []byte
}

// NewSigner creates a new
func NewSigner(privateKey []byte) *signer {
	return &signer{
		privateKey: privateKey,
	}
}

// ID returns the ID of a signer
func (s *signer) ID() string {
	return s.id
}

// Sign signs a message with the private key
func (s *signer) Sign(message []byte) ([]byte, error) {
	if s.privateKey == nil {
		return nil, ErrNilPrivateKey
	}

	if message == nil {
		return nil, ErrNilMessage
	}

	return ed25519.Sign(s.privateKey, message), nil
}

// Verify verifies a signature of a message using the public key
func (s *signer) Verify(publicKey []byte, message []byte, signature []byte) error {
	if publicKey == nil {
		return ErrNilPublicKey
	}

	if message == nil {
		return ErrNilMessage
	}

	if signature == nil {
		return ErrNilSignature
	}

	ok := ed25519.Verify(publicKey, message, signature)
	if !ok {
		return ErrWrongSignature
	}

	return nil
}

// IsInterfaceNil checks for nil interface
func (s *signer) IsInterfaceNil() bool {
	return s == nil
}
