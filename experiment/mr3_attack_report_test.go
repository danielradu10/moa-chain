package experiment

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteMR3AttackReport_DistinguishesGenerationAndEvaluation(t *testing.T) {
	dir := t.TempDir()
	agentsDir := filepath.Join(dir, "agents")
	require.NoError(t, os.MkdirAll(agentsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "proposer.jsonl"), []byte(
		`{"validator_id":"validator-10","provider":"deepseek","model":"deepseek-v4-pro","operation":"synthesize","tx_hash":"abc","latency_ms":12.5,"request_payload":{"correct_answers":["honest one","honest two"],"synthesis_prompt_version":"byzantine_synthesizer_v1","synthesis_system_prompt":"attack prompt"},"parsed_response":{"synthesized_answer":"adaptive output"},"input_tokens":100,"output_tokens":20,"total_tokens":120,"success":true}`+"\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(agentsDir, "evaluator.jsonl"), []byte(
		`{"validator_id":"validator-1","provider":"openai","model":"gpt-5.4-mini","operation":"evaluate_synthesis","tx_hash":"abc","latency_ms":7.5,"request_payload":{},"parsed_response":{"approved":false},"input_tokens":80,"output_tokens":5,"total_tokens":85,"success":true}`+"\n"), 0o644))

	cfg := Config{ForcedMR3Proposer: "validator-10", Validators: make([]ValidatorSpec, 10)}
	require.NoError(t, WriteMR3AttackReport(dir, cfg, "abc", "", false))

	var report MR3AttackReport
	raw, err := os.ReadFile(filepath.Join(dir, "mr3-attack.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &report))
	assert.Equal(t, "requires_review", report.AttackGenerationStatus)
	assert.Equal(t, []string{"honest one", "honest two"}, report.CorrectAnswers)
	assert.Equal(t, "attack prompt", report.ByzantinePrompt)
	assert.Equal(t, "adaptive output", report.RawSynthesis)
	require.Len(t, report.Evaluators, 1)
	assert.False(t, report.Evaluators[0].Approved)
	assert.Equal(t, 0, report.Approvals)
	assert.Equal(t, 1, report.Rejections)
	assert.Equal(t, 6, report.ApprovalQuorum)
	assert.False(t, report.QuorumReached)
	assert.Equal(t, "waiting_for_approval_quorum_or_timeout", report.MR3Status)
}
