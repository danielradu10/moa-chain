package experiment

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// MR3AttackEvaluator records one normal evaluator's decision from its agent trace.
type MR3AttackEvaluator struct {
	ValidatorID  string  `json:"validator_id"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	Approved     bool    `json:"approved"`
	LatencyMS    float64 `json:"latency_ms"`
	InputTokens  *int    `json:"input_tokens"`
	OutputTokens *int    `json:"output_tokens"`
	TotalTokens  *int    `json:"total_tokens"`
}

// MR3AttackReport is a human-review-oriented summary derived only from recorded
// agent traces and terminal protocol state. It does not judge attack quality.
type MR3AttackReport struct {
	ForcedProposerID       string               `json:"forced_proposer_validator_id"`
	ProposerProvider       string               `json:"proposer_provider"`
	ProposerModel          string               `json:"proposer_model"`
	CorrectAnswers         []string             `json:"correct_answers"`
	ByzantinePromptVersion string               `json:"byzantine_synthesis_prompt_version"`
	ByzantinePrompt        string               `json:"byzantine_synthesis_prompt"`
	RawSynthesis           string               `json:"raw_synthesis"`
	SynthesisLatencyMS     float64              `json:"synthesis_latency_ms"`
	SynthesisInputTokens   *int                 `json:"synthesis_input_tokens"`
	SynthesisOutputTokens  *int                 `json:"synthesis_output_tokens"`
	SynthesisTotalTokens   *int                 `json:"synthesis_total_tokens"`
	AttackGenerationStatus string               `json:"attack_generation_status"`
	Evaluators             []MR3AttackEvaluator `json:"evaluators"`
	Approvals              int                  `json:"approvals"`
	Rejections             int                  `json:"rejections"`
	ApprovalQuorum         int                  `json:"approval_quorum"`
	QuorumReached          bool                 `json:"quorum_reached"`
	MR3Status              string               `json:"mr3_status"`
	FinalTransactionStatus string               `json:"final_transaction_status"`
}

type agentTraceRecord struct {
	ValidatorID string          `json:"validator_id"`
	Provider    string          `json:"provider"`
	Model       string          `json:"model"`
	Operation   string          `json:"operation"`
	TxHash      string          `json:"tx_hash"`
	LatencyMS   float64         `json:"latency_ms"`
	Request     json.RawMessage `json:"request_payload"`
	Response    json.RawMessage `json:"parsed_response"`
	Input       *int            `json:"input_tokens"`
	Output      *int            `json:"output_tokens"`
	Total       *int            `json:"total_tokens"`
	Success     bool            `json:"success"`
}

// WriteMR3AttackReport derives mr3-attack.json from the run's authoritative
// traces. A generated synthesis is marked requires_review; semantic success is
// intentionally never inferred automatically.
func WriteMR3AttackReport(dir string, cfg Config, txHash, finalStatus string, mr3Finalized bool) error {
	report := MR3AttackReport{
		ForcedProposerID:       cfg.ForcedMR3Proposer,
		AttackGenerationStatus: "attack_generation_failed_or_inconclusive",
		ApprovalQuorum:         (2 * len(cfg.Validators)) / 3,
		MR3Status:              "waiting_for_approval_quorum_or_timeout",
		FinalTransactionStatus: finalStatus,
	}

	entries, err := os.ReadDir(filepath.Join(dir, "agents"))
	if err != nil {
		return fmt.Errorf("read MR3 agent traces: %w", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		if err := readMR3Trace(filepath.Join(dir, "agents", entry.Name()), txHash, &report); err != nil {
			return err
		}
	}

	report.QuorumReached = report.Approvals >= report.ApprovalQuorum
	if mr3Finalized {
		report.MR3Status = "finalized"
	}
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal MR3 attack report: %w", err)
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(dir, "mr3-attack.json"), raw, 0o644)
}

func readMR3Trace(path, txHash string, report *MR3AttackReport) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open MR3 trace: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record agentTraceRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return fmt.Errorf("parse MR3 trace %s: %w", path, err)
		}
		if record.TxHash != txHash || !record.Success {
			continue
		}
		switch record.Operation {
		case "synthesize":
			if record.ValidatorID != report.ForcedProposerID {
				continue
			}
			var request struct {
				CorrectAnswers []string `json:"correct_answers"`
				PromptVersion  string   `json:"synthesis_prompt_version"`
				SystemPrompt   string   `json:"synthesis_system_prompt"`
			}
			var response struct {
				Synthesis string `json:"synthesized_answer"`
			}
			_ = json.Unmarshal(record.Request, &request)
			_ = json.Unmarshal(record.Response, &response)
			report.ProposerProvider = record.Provider
			report.ProposerModel = record.Model
			report.CorrectAnswers = request.CorrectAnswers
			report.ByzantinePromptVersion = request.PromptVersion
			report.ByzantinePrompt = request.SystemPrompt
			report.RawSynthesis = response.Synthesis
			report.SynthesisLatencyMS = record.LatencyMS
			report.SynthesisInputTokens = record.Input
			report.SynthesisOutputTokens = record.Output
			report.SynthesisTotalTokens = record.Total
			if response.Synthesis != "" {
				report.AttackGenerationStatus = "requires_review"
			}
		case "evaluate_synthesis":
			var response struct {
				Approved bool `json:"approved"`
			}
			if err := json.Unmarshal(record.Response, &response); err != nil {
				continue
			}
			report.Evaluators = append(report.Evaluators, MR3AttackEvaluator{
				ValidatorID: record.ValidatorID, Provider: record.Provider, Model: record.Model,
				Approved: response.Approved, LatencyMS: record.LatencyMS,
				InputTokens: record.Input, OutputTokens: record.Output, TotalTokens: record.Total,
			})
			if response.Approved {
				report.Approvals++
			} else {
				report.Rejections++
			}
		}
	}
	return scanner.Err()
}
