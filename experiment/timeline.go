package experiment

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Event is one entry in timeline.jsonl.
type Event struct {
	Timestamp   string         `json:"ts"`
	EventType   string         `json:"event"`
	RunID       string         `json:"run_id"`
	TxHash      string         `json:"tx_hash,omitempty"`
	Epoch       uint64         `json:"epoch,omitempty"`
	Round       uint64         `json:"round,omitempty"`
	MiniRound   uint64         `json:"mini_round,omitempty"`
	ValidatorID string         `json:"validator_id,omitempty"`
	Details     map[string]any `json:"details,omitempty"`
}

// Timeline writes events to timeline.jsonl in an experiment directory.
type Timeline struct {
	mu    sync.Mutex
	file  *os.File
	runID string
}

// NewTimeline creates (or opens for append) {dir}/timeline.jsonl.
func NewTimeline(dir, runID string) (*Timeline, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, "timeline.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open timeline: %w", err)
	}
	return &Timeline{file: f, runID: runID}, nil
}

// Emit appends one event to the timeline. Thread-safe.
func (t *Timeline) Emit(e Event) {
	e.RunID = t.runID
	if e.Timestamp == "" {
		e.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	line, _ := json.Marshal(e)
	line = append(line, '\n')
	t.mu.Lock()
	defer t.mu.Unlock()
	_, _ = t.file.Write(line)
}

// Close flushes and closes the underlying file.
func (t *Timeline) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.file.Close()
}
