package httpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"time"

	"moa-chain/agent"
	"moa-chain/data"
)

// Config holds the connection parameters and expected prompt identifiers for
// the Python agent service. Empty prompt version / hash strings skip the
// corresponding response verification check.
//
// Per-operation timeouts (LabelTimeoutSeconds, AnswerTimeoutSeconds,
// JudgeTimeoutSeconds) override TimeoutSeconds for their respective endpoints.
// Set TimeoutSeconds as the default and override only the operations that need
// a different budget.
type Config struct {
	BaseURL        string
	TimeoutSeconds int // fallback for any operation that has no specific timeout set

	LabelTimeoutSeconds  int // 0 = use TimeoutSeconds
	AnswerTimeoutSeconds int // 0 = use TimeoutSeconds
	JudgeTimeoutSeconds  int // 0 = use TimeoutSeconds

	// These are compared against the returned prompt_version / prompt_hash in
	// every /label and /answer response as a defense-in-depth check.
	LabelPromptVersion  string
	LabelPromptHash     string
	AnswerPromptVersion string
	AnswerPromptHash    string
}

// Client implements agent.BatchAgent by calling the Python service over HTTP.
type Client struct {
	labelHTTP         http.Client
	answerHTTP        http.Client
	judgeHTTP         http.Client
	config            Config
	allowedSubdomains []string // sorted; derived from data.PossibleSubDomains at construction
}

var _ agent.BatchAgent = (*Client)(nil)

func operationTimeout(specific, fallback int) time.Duration {
	if specific > 0 {
		return time.Duration(specific) * time.Second
	}
	return time.Duration(fallback) * time.Second
}

// New builds a Client from cfg. The allowed subdomain list is derived once from
// data.PossibleSubDomains and sorted for deterministic request serialization.
func New(cfg Config) *Client {
	subdomains := make([]string, 0, len(data.PossibleSubDomains))
	for s := range data.PossibleSubDomains {
		subdomains = append(subdomains, s)
	}
	sort.Strings(subdomains)

	return &Client{
		labelHTTP:         http.Client{Timeout: operationTimeout(cfg.LabelTimeoutSeconds, cfg.TimeoutSeconds)},
		answerHTTP:        http.Client{Timeout: operationTimeout(cfg.AnswerTimeoutSeconds, cfg.TimeoutSeconds)},
		judgeHTTP:         http.Client{Timeout: operationTimeout(cfg.JudgeTimeoutSeconds, cfg.TimeoutSeconds)},
		config:            cfg,
		allowedSubdomains: subdomains,
	}
}

// LabelBatch sends all transactions to POST /label in a single call and maps
// the response to []agent.LabelResult.
func (c *Client) LabelBatch(txs []data.Transaction) ([]agent.LabelResult, error) {
	inputs := make([]labelTxInput, len(txs))
	for i, tx := range txs {
		inputs[i] = labelTxInput{
			TxHash: string(tx.GetTxHash()),
			Prompt: string(tx.GetPrompt()),
		}
	}

	req := labelRequest{
		PromptVersion:     c.config.LabelPromptVersion,
		AllowedSubdomains: c.allowedSubdomains,
		Transactions:      inputs,
	}

	var resp labelResponse
	if err := c.post(&c.labelHTTP, "/label", req, &resp); err != nil {
		return nil, err
	}

	if err := c.verifyPrompt(resp.PromptVersion, c.config.LabelPromptVersion,
		resp.PromptHash, c.config.LabelPromptHash); err != nil {
		return nil, err
	}

	results := make([]agent.LabelResult, len(resp.Results))
	for i, r := range resp.Results {
		labels := make([]string, len(r.Labels))
		for j, e := range r.Labels {
			labels[j] = e.Subdomain
		}
		results[i] = agent.LabelResult{
			TxHash: []byte(r.TxHash),
			Labels: labels,
		}
	}

	return results, nil
}

// AnswerBatch sends all transactions to POST /answer in a single call and maps
// the response to []agent.AnswerResult.
func (c *Client) AnswerBatch(txs []data.Transaction) ([]agent.AnswerResult, error) {
	inputs := make([]answerTxInput, len(txs))
	for i, tx := range txs {
		inputs[i] = answerTxInput{
			TxHash:     string(tx.GetTxHash()),
			Prompt:     string(tx.GetPrompt()),
			Subdomains: tx.GetDomainLabels(),
		}
	}

	req := answerRequest{
		PromptVersion: c.config.AnswerPromptVersion,
		Transactions:  inputs,
	}

	var resp answerResponse
	if err := c.post(&c.answerHTTP, "/answer", req, &resp); err != nil {
		return nil, err
	}

	if err := c.verifyPrompt(resp.PromptVersion, c.config.AnswerPromptVersion,
		resp.PromptHash, c.config.AnswerPromptHash); err != nil {
		return nil, err
	}

	results := make([]agent.AnswerResult, len(resp.Results))
	for i, r := range resp.Results {
		results[i] = agent.AnswerResult{
			TxHash: []byte(r.TxHash),
			Answer: r.Answer,
		}
	}

	return results, nil
}

// JudgeTransactionAnswers sends the Go-built prompt strings to POST /judge
// and returns the raw model output string for the Go side to parse.
func (c *Client) JudgeTransactionAnswers(request agent.AnswerJudgeRequest) (string, error) {
	req := judgeRequest{
		SystemPrompt: request.SystemPrompt,
		UserPrompt:   request.UserPrompt,
	}

	var resp judgeResponseJSON
	if err := c.post(&c.judgeHTTP, "/judge", req, &resp); err != nil {
		return "", err
	}

	return resp.Response, nil
}

func (c *Client) post(hc *http.Client, path string, body any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("httpclient: marshal request: %w", err)
	}

	resp, err := hc.Post(c.config.BaseURL+path, "application/json", bytes.NewReader(encoded))
	if err != nil {
		if isTimeout(err) {
			return ErrProviderTimeout
		}
		return fmt.Errorf("%w: %w", ErrHTTPError, err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("httpclient: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var pyErr pythonErrorResponse
		if jsonErr := json.Unmarshal(respBytes, &pyErr); jsonErr == nil && pyErr.ErrorCode != "" {
			return mapPythonError(pyErr.ErrorCode, pyErr.Message)
		}

		return fmt.Errorf("%w: status %d", ErrHTTPError, resp.StatusCode)
	}

	if err := json.Unmarshal(respBytes, out); err != nil {
		return fmt.Errorf("httpclient: unmarshal response: %w", err)
	}

	return nil
}

// verifyPrompt checks the returned prompt_version and prompt_hash against the
// expected values from Config. An empty expected value skips that check.
func (c *Client) verifyPrompt(gotVersion, wantVersion, gotHash, wantHash string) error {
	if wantVersion != "" && gotVersion != wantVersion {
		return fmt.Errorf("%w: got %q, want %q", ErrPromptVersionMismatch, gotVersion, wantVersion)
	}
	if wantHash != "" && gotHash != wantHash {
		return fmt.Errorf("%w: got %q, want %q", ErrPromptHashMismatch, gotHash, wantHash)
	}
	return nil
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}
