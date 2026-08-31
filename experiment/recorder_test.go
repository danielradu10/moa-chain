package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateRunID_IsUnique(t *testing.T) {
	id1 := GenerateRunID()
	id2 := GenerateRunID()
	assert.Len(t, id1, 32)
	assert.NotEqual(t, id1, id2)
}

func TestGenerateRunID_IsHex(t *testing.T) {
	id := GenerateRunID()
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("non-hex character %q in run ID %q", c, id)
		}
	}
}

func TestWriteManifest_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	m := Manifest{
		RunID:             "abc123",
		StartTimestamp:    time.Now().UTC().Format(time.RFC3339),
		NumNodes:          10,
		CommitteeStrategy: "full",
		Quorum:            7,
		MiniRoundDuration: "15s",
		CanonicalQuestion: "test question",
		Validators: []ValidatorSpec{
			{
				ValidatorID:   "validator-1",
				ValidatorName: "gpt-5.4-mini-1",
				Provider:      "openai",
				Model:         "gpt-5.4-mini",
				AgentEndpoint: "http://127.0.0.1:8100",
			},
		},
	}
	require.NoError(t, WriteManifest(dir, m))

	raw, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	require.NoError(t, err)

	var got Manifest
	require.NoError(t, json.Unmarshal(raw, &got))
	assert.Equal(t, "abc123", got.RunID)
	assert.Equal(t, 10, got.NumNodes)
	assert.Equal(t, "full", got.CommitteeStrategy)
	assert.Equal(t, 7, got.Quorum)
	assert.Len(t, got.Validators, 1)
	assert.Equal(t, "gpt-5.4-mini-1", got.Validators[0].ValidatorName)
	assert.Equal(t, "openai", got.Validators[0].Provider)
}

func TestWriteManifest_CreatesParentDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nested", "run-dir")
	m := Manifest{RunID: "r1", NumNodes: 1, Validators: []ValidatorSpec{{ValidatorID: "v-1"}}}
	require.NoError(t, WriteManifest(dir, m))
	assert.FileExists(t, filepath.Join(dir, "manifest.json"))
}

func TestBuildManifest_PopulatesFields(t *testing.T) {
	cfg := Config{
		Validators:                   []ValidatorSpec{{ValidatorID: "v-1"}},
		CommitteeStrategy:            "full",
		Quorum:                       7,
		MiniRoundDurationStr:         "30s",
		ClassificationGracePeriodStr: "10s",
		Question:                     "write a sort function",
	}
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := BuildManifest("myrun", cfg, start)

	assert.Equal(t, "myrun", m.RunID)
	assert.Equal(t, "2026-01-01T00:00:00Z", m.StartTimestamp)
	assert.Equal(t, 1, m.NumNodes)
	assert.Equal(t, "full", m.CommitteeStrategy)
	assert.Equal(t, 7, m.Quorum)
	assert.Equal(t, "30s", m.MiniRoundDuration)
	assert.Equal(t, "10s", m.ClassificationGracePeriod)
	assert.Equal(t, "write a sort function", m.CanonicalQuestion)
}

func TestTimeline_EmitAndRead(t *testing.T) {
	dir := t.TempDir()
	tl, err := NewTimeline(dir, "run-test")
	require.NoError(t, err)
	defer tl.Close()

	tl.Emit(Event{EventType: "tx_submitted", TxHash: "abc"})
	tl.Emit(Event{EventType: "mr1_started", Round: 5})

	raw, err := os.ReadFile(filepath.Join(dir, "timeline.jsonl"))
	require.NoError(t, err)

	lines := splitNonEmpty(string(raw))
	require.Len(t, lines, 2)

	var e1 Event
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &e1))
	assert.Equal(t, "tx_submitted", e1.EventType)
	assert.Equal(t, "run-test", e1.RunID)
	assert.Equal(t, "abc", e1.TxHash)
	assert.NotEmpty(t, e1.Timestamp)

	var e2 Event
	require.NoError(t, json.Unmarshal([]byte(lines[1]), &e2))
	assert.Equal(t, "mr1_started", e2.EventType)
	assert.Equal(t, uint64(5), e2.Round)
}

func TestTimeline_RunIDInjected(t *testing.T) {
	dir := t.TempDir()
	tl, err := NewTimeline(dir, "injected-id")
	require.NoError(t, err)
	defer tl.Close()

	tl.Emit(Event{EventType: "ping"})

	raw, _ := os.ReadFile(filepath.Join(dir, "timeline.jsonl"))
	var e Event
	require.NoError(t, json.Unmarshal(raw[:len(raw)-1], &e))
	assert.Equal(t, "injected-id", e.RunID)
}

func TestLoadConfig_ParsesDuration(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{
		"validators": [{"validator_id":"v-1","validator_name":"gpt-5.4-mini-1","provider":"openai","model":"gpt-5.4-mini","agent_endpoint":"http://127.0.0.1:8100"}],
		"committee_strategy": "full",
		"quorum": 7,
		"mini_round_duration": "15s",
		"vote_collection_deadline": "5s",
		"classification_grace_period": "10s",
		"start_round": 2,
		"question": "test"
	}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, 15*time.Second, cfg.MiniRoundDuration)
	assert.Equal(t, 5*time.Second, cfg.VoteCollectionDeadline)
	assert.Equal(t, 10*time.Second, cfg.ClassificationGracePeriod)
	assert.Equal(t, "full", cfg.CommitteeStrategy)
	assert.Equal(t, 7, cfg.Quorum)
	assert.Len(t, cfg.Validators, 1)
	assert.Equal(t, "gpt-5.4-mini-1", cfg.Validators[0].ValidatorName)
}

func TestLoadConfig_PreservesMockPreprocessing(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{"validators":[{"validator_id":"validator-7","validator_name":"mocked-agent","mock_preprocessing":{"label":"systems_programming","answer":"wrong answer"}}]}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	require.NotNil(t, cfg.Validators[0].MockPreprocessing)
	assert.Equal(t, "systems_programming", cfg.Validators[0].MockPreprocessing.Label)
	assert.Equal(t, "wrong answer", cfg.Validators[0].MockPreprocessing.Answer)
}

func TestLoadConfig_PreservesThreeDistinctMockValidators(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{"validators":[
		{"validator_id":"validator-4","provider":"mock","model":"mocked-agent","mock_preprocessing":{"label":"systems_programming","answer":"wrong four"},"mock_judge_correct_answers":["wrong four","wrong seven","wrong eight"]},
		{"validator_id":"validator-7","provider":"mock","model":"mocked-agent","mock_preprocessing":{"label":"systems_programming","answer":"wrong seven"},"mock_judge_correct_answers":["wrong four","wrong seven","wrong eight"]},
		{"validator_id":"validator-8","provider":"mock","model":"mocked-agent","mock_preprocessing":{"label":"systems_programming","answer":"wrong eight"},"mock_judge_correct_answers":["wrong four","wrong seven","wrong eight"]}
	]}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	require.Len(t, cfg.Validators, 3)
	for i, want := range []struct{ id, answer string }{
		{"validator-4", "wrong four"},
		{"validator-7", "wrong seven"},
		{"validator-8", "wrong eight"},
	} {
		assert.Equal(t, want.id, cfg.Validators[i].ValidatorID)
		assert.Equal(t, want.answer, cfg.Validators[i].MockPreprocessing.Answer)
		assert.Equal(t, []string{"wrong four", "wrong seven", "wrong eight"}, cfg.Validators[i].MockJudgeCorrectAnswers)
	}
}

func TestLoadConfig_PreservesForcedByzantineMR3Proposer(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{
		"forced_mr3_proposer":"validator-10",
		"validators":[
			{"validator_id":"validator-1","provider":"openai","model":"gpt-5.4-mini"},
			{"validator_id":"validator-10","provider":"deepseek","model":"deepseek-v4-pro","byzantine_mr3_synthesis":true}
		]
	}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, "validator-10", cfg.ForcedMR3Proposer)
	assert.False(t, cfg.Validators[0].ByzantineMR3Synthesis)
	assert.True(t, cfg.Validators[1].ByzantineMR3Synthesis)

	manifest := BuildManifest("run", cfg, time.Unix(0, 0))
	assert.Equal(t, "validator-10", manifest.ForcedMR3Proposer)
}

func TestLoadConfig_DefaultDoesNotForceMR3Proposer(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{"validators":[{"validator_id":"validator-1"}]}`), 0o644))

	cfg, err := LoadConfig(cfgPath)
	require.NoError(t, err)
	assert.Empty(t, cfg.ForcedMR3Proposer)
	assert.False(t, cfg.Validators[0].ByzantineMR3Synthesis)
}

func TestLoadConfig_RejectsMismatchedByzantineMR3Proposer(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{
		"forced_mr3_proposer":"validator-1",
		"validators":[{"validator_id":"validator-1"},{"validator_id":"validator-2","byzantine_mr3_synthesis":true}]
	}`), 0o644))

	_, err := LoadConfig(cfgPath)
	assert.ErrorContains(t, err, "must match forced_mr3_proposer")
}

func TestLoadConfig_EmptyValidatorsError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`{"validators":[]}`), 0o644))

	_, err := LoadConfig(cfgPath)
	assert.ErrorContains(t, err, "no validators")
}

func TestLoadConfig_BadDurationError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{"validators":[{"validator_id":"v-1"}],"mini_round_duration":"notaduration"}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	_, err := LoadConfig(cfgPath)
	assert.ErrorContains(t, err, "mini_round_duration")
}

func TestLoadConfig_BadClassificationGracePeriodError(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "cfg.json")
	content := `{"validators":[{"validator_id":"v-1"}],"classification_grace_period":"notaduration"}`
	require.NoError(t, os.WriteFile(cfgPath, []byte(content), 0o644))

	_, err := LoadConfig(cfgPath)
	assert.ErrorContains(t, err, "classification_grace_period")
}

// splitNonEmpty splits s on newlines and discards empty lines.
func splitNonEmpty(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}
