package signing

import (
	"errors"
)

// ErrNilPublicKey signals a nil public key
var ErrNilPublicKey = errors.New("signing error: public key is nil")

// ErrNilMessage signals a nil message
var ErrNilMessage = errors.New("signing error: message is nil")

// ErrNilSignature signals a nil signature
var ErrNilSignature = errors.New("signing error: signature is nil")

// ErrNilPrivateKey signals a nil private key
var ErrNilPrivateKey = errors.New("signing error: private key is nil")

// ErrWrongSignature signals a wrong signature
var ErrWrongSignature = errors.New("signing error: signature is wrong")
