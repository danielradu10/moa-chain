package httpclient

// ── /label ───────────────────────────────────────────────────────────────────

type labelTxInput struct {
	TxHash string `json:"tx_hash"`
	Prompt string `json:"prompt"`
}

type labelRequest struct {
	PromptVersion     string         `json:"prompt_version"`
	AllowedSubdomains []string       `json:"allowed_subdomains"`
	Transactions      []labelTxInput `json:"transactions"`
}

type labelEntry struct {
	Subdomain  string  `json:"subdomain"`
	Confidence float64 `json:"confidence"`
}

type labelResultJSON struct {
	TxHash string       `json:"tx_hash"`
	Labels []labelEntry `json:"labels"`
}

type labelResponse struct {
	PromptVersion string            `json:"prompt_version"`
	PromptHash    string            `json:"prompt_hash"`
	Results       []labelResultJSON `json:"results"`
}

// ── /answer ──────────────────────────────────────────────────────────────────

type answerTxInput struct {
	TxHash     string   `json:"tx_hash"`
	Prompt     string   `json:"prompt"`
	Subdomains []string `json:"subdomains"`
}

type answerRequest struct {
	PromptVersion string          `json:"prompt_version"`
	Transactions  []answerTxInput `json:"transactions"`
}

type answerResultJSON struct {
	TxHash string `json:"tx_hash"`
	Answer string `json:"answer"`
}

type answerResponse struct {
	PromptVersion string             `json:"prompt_version"`
	PromptHash    string             `json:"prompt_hash"`
	Results       []answerResultJSON `json:"results"`
}

// ── /judge ───────────────────────────────────────────────────────────────────

type judgeRequest struct {
	SystemPrompt string `json:"system_prompt"`
	UserPrompt   string `json:"user_prompt"`
}

type judgeResponseJSON struct {
	Response string `json:"response"`
}

// ── error response ───────────────────────────────────────────────────────────

type pythonErrorResponse struct {
	ErrorCode string `json:"error"`
	Message   string `json:"detail"`
}
