package explorer

// HealthResponse is returned by GET /api/v1/health.
type HealthResponse struct {
	Status         string `json:"status"`
	ChainLength    uint64 `json:"chain_length"`
	CurrentRound   uint64 `json:"current_round"`
	CurrentMiniRound uint64 `json:"current_mini_round"`
	CurrentEpoch   uint64 `json:"current_epoch"`
}

// ErrorResponse is the standard error envelope for all 4xx/5xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
