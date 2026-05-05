package validators

type Validator struct {
	publicID    string
	publicKey   []byte
	globalScore float64
}

func NewValidator(
	publicID string,
	publicKey []byte,
	globalScore uint64,
) *Validator {
	return &Validator{}
}

func (v *Validator) PublicID() string {
	return v.publicID
}

func (v *Validator) PublicKey() []byte {
	return v.publicKey
}

func (v *Validator) GlobalScore() float64 {
	return v.globalScore
}

func (v *Validator) SetGlobalScore(globalScore float64) {
	v.globalScore = globalScore
}

func (v *Validator) SetPublicID(publicID string) {
	v.publicID = publicID
}

func (v *Validator) SetPublicKey(publicKey []byte) {
	v.publicKey = publicKey
}
