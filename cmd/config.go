package cmd

type MempoolConfig struct{}

// AgentConfig holds configuration for the HTTP agent client that connects Go
// validator nodes to the Python LLM service.
type AgentConfig struct {
	// BaseURL is the base URL of the Python agent service (e.g. http://127.0.0.1:8081).
	BaseURL string
	// TimeoutSeconds is the per-request HTTP timeout for all agent calls.
	TimeoutSeconds int
	// LabelPromptVersion and LabelPromptHash are checked against every /label
	// response. Leave empty to skip the check (useful during development).
	LabelPromptVersion string
	LabelPromptHash    string
	// AnswerPromptVersion and AnswerPromptHash are checked against every /answer
	// response. Leave empty to skip the check (useful during development).
	AnswerPromptVersion string
	AnswerPromptHash    string
}

// NodeConfig holds the complete node configuration.
type NodeConfig struct {
	Mempool MempoolConfig
	Agent   AgentConfig
}

// DefaultNodeConfig returns a NodeConfig with sensible production defaults.
// Individual fields can be overridden by reading environment variables before
// passing the config to the node constructor.
func DefaultNodeConfig() NodeConfig {
	return NodeConfig{
		Agent: AgentConfig{
			BaseURL:             "http://127.0.0.1:8081",
			TimeoutSeconds:      60,
			LabelPromptVersion:  "labeler_v1",
			AnswerPromptVersion: "answerer_v1",
			// Prompt hashes are left empty by default so the client skips the
			// hash verification check. Pin them in production deployments.
		},
	}
}
