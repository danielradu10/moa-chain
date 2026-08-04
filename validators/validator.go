package validators

// SubdomainsScores defines the map of scores for a validator.
// The map shows how well behaves a validator on specific subdomains.
type SubdomainsScores map[string]int

// Validator is the data structure which encapsulates info about a specific validator.
type Validator struct {
	publicID        string
	publicKey       []byte
	globalScore     float64
	subdomainScores SubdomainsScores
}

func NewValidator(
	publicID string,
	publicKey []byte,
	globalScore uint64,
) *Validator {
	return &Validator{
		publicID:    publicID,
		publicKey:   publicKey,
		globalScore: float64(globalScore),
	}
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

// SetSubdomainScores sets the subdomain scores of a validator.
func (v *Validator) SetSubdomainScores(subdomainScores SubdomainsScores) {
	v.subdomainScores = subdomainScores
}

// SubdomainScores returns the subdomain scores of a validator.
func (v *Validator) SubdomainScores() SubdomainsScores {
	return v.subdomainScores
}
