package experiment

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest is written once at experiment startup to {dir}/manifest.json.
type Manifest struct {
	RunID                     string          `json:"run_id"`
	StartTimestamp            string          `json:"start_timestamp"`
	NumNodes                  int             `json:"num_nodes"`
	CommitteeStrategy         string          `json:"committee_strategy"`
	Quorum                    int             `json:"quorum"`
	MiniRoundDuration         string          `json:"mini_round_duration"`
	VoteCollectionDeadline    string          `json:"vote_collection_deadline"`
	ClassificationGracePeriod string          `json:"classification_grace_period"`
	CanonicalQuestion         string          `json:"canonical_question"`
	Validators                []ValidatorSpec `json:"validators"`
}

// GenerateRunID returns a random 32-hex-character run identifier.
func GenerateRunID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("experiment: rand.Read failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// BuildManifest constructs a Manifest from a Config and run metadata.
func BuildManifest(runID string, cfg Config, startTime time.Time) Manifest {
	dur := cfg.MiniRoundDurationStr
	if dur == "" && cfg.MiniRoundDuration > 0 {
		dur = cfg.MiniRoundDuration.String()
	}
	return Manifest{
		RunID:                     runID,
		StartTimestamp:            startTime.UTC().Format(time.RFC3339),
		NumNodes:                  len(cfg.Validators),
		CommitteeStrategy:         cfg.CommitteeStrategy,
		Quorum:                    cfg.Quorum,
		MiniRoundDuration:         dur,
		VoteCollectionDeadline:    cfg.VoteCollectionDeadlineStr,
		ClassificationGracePeriod: cfg.ClassificationGracePeriodStr,
		CanonicalQuestion:         cfg.Question,
		Validators:                cfg.Validators,
	}
}

// WriteManifest serialises m to {dir}/manifest.json.
func WriteManifest(dir string, m Manifest) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	raw = append(raw, '\n')
	path := filepath.Join(dir, "manifest.json")
	return os.WriteFile(path, raw, 0o644)
}
