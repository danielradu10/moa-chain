package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// ValidatorSpec is the fixed identity of one validator in a heterogeneous experiment.
type ValidatorSpec struct {
	ValidatorID             string                 `json:"validator_id"`
	ValidatorName           string                 `json:"validator_name"`
	Provider                string                 `json:"provider"`
	Model                   string                 `json:"model"`
	AgentEndpoint           string                 `json:"agent_endpoint"`
	MockPreprocessing       *MockPreprocessingSpec `json:"mock_preprocessing,omitempty"`
	MockJudgeCorrectAnswers []string               `json:"mock_judge_correct_answers,omitempty"`
}

// MockPreprocessingSpec configures deterministic label and answer responses for
// a controlled experiment validator. Other mocked operations are implemented by
// the configured local mock agent service.
type MockPreprocessingSpec struct {
	Label  string `json:"label"`
	Answer string `json:"answer"`
}

// Config holds all parameters for one experiment run.
type Config struct {
	Validators                   []ValidatorSpec `json:"validators"`
	CommitteeStrategy            string          `json:"committee_strategy"` // "full" or "half"
	Quorum                       int             `json:"quorum"`
	MiniRoundDurationStr         string          `json:"mini_round_duration"`         // e.g. "15s"
	VoteCollectionDeadlineStr    string          `json:"vote_collection_deadline"`    // e.g. "5s"
	ClassificationGracePeriodStr string          `json:"classification_grace_period"` // e.g. "10s"
	StartRound                   uint64          `json:"start_round"`
	Question                     string          `json:"question"`

	// Parsed from MiniRoundDurationStr; populated by LoadConfig.
	MiniRoundDuration         time.Duration `json:"-"`
	VoteCollectionDeadline    time.Duration `json:"-"`
	ClassificationGracePeriod time.Duration `json:"-"`
}

// LoadConfig reads an experiment config JSON from path.
func LoadConfig(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("load config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.MiniRoundDurationStr != "" {
		d, err := time.ParseDuration(cfg.MiniRoundDurationStr)
		if err != nil {
			return Config{}, fmt.Errorf("parse mini_round_duration %q: %w", cfg.MiniRoundDurationStr, err)
		}
		cfg.MiniRoundDuration = d
	}
	if cfg.VoteCollectionDeadlineStr != "" {
		d, err := time.ParseDuration(cfg.VoteCollectionDeadlineStr)
		if err != nil {
			return Config{}, fmt.Errorf("parse vote_collection_deadline %q: %w", cfg.VoteCollectionDeadlineStr, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("vote_collection_deadline must be positive")
		}
		cfg.VoteCollectionDeadline = d
	}
	if cfg.ClassificationGracePeriodStr != "" {
		d, err := time.ParseDuration(cfg.ClassificationGracePeriodStr)
		if err != nil {
			return Config{}, fmt.Errorf("parse classification_grace_period %q: %w", cfg.ClassificationGracePeriodStr, err)
		}
		if d <= 0 {
			return Config{}, fmt.Errorf("classification_grace_period must be positive")
		}
		cfg.ClassificationGracePeriod = d
	}
	if len(cfg.Validators) == 0 {
		return Config{}, fmt.Errorf("config has no validators")
	}
	return cfg, nil
}
