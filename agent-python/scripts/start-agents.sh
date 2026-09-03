#!/usr/bin/env bash
# Start all 10 heterogeneous agent servers for an experiment run.
#
# Usage:
#   EXPERIMENT_DIR=/path/to/run \
#   OPENAI_API_KEY=...          \
#   ANTHROPIC_API_KEY=...       \
#   GEMINI_API_KEY=...          \
#   DEEPSEEK_API_KEY=...        \
#   ./scripts/start-agents.sh
#
# The script starts one uvicorn process per validator, waits for Ctrl-C,
# then shuts all agents down cleanly.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
EXPERIMENT_DIR="${EXPERIMENT_DIR:-}"
LOG_DIR="${LOG_DIR:-${EXPERIMENT_DIR}/logs}"
EXPERIMENT_CONFIG="${EXPERIMENT_CONFIG:-}"
USE_VALIDATOR_NAME_AS_ID="${USE_VALIDATOR_NAME_AS_ID:-false}"

PIDS=()

cleanup() {
    echo ""
    echo "Stopping agent servers..."
    for pid in "${PIDS[@]}"; do
        kill "$pid" 2>/dev/null || true
    done
    wait 2>/dev/null || true
    echo "All agents stopped."
}
trap cleanup EXIT INT TERM

start_agent() {
    local port="$1"
    local provider="$2"
    local model_env_var="$3"
    local model_value="$4"
    local validator_id="$5"
    local validator_name="$6"
    local mock_label="${7:-}"
    local mock_answer="${8:-}"
    local agent_provider="${9:-$provider}"
    local agent_model="${10:-$model_value}"
    local mock_judge_correct_answers="${11:-}"
    local byzantine_mr3_synthesis="${12:-false}"

	if [ "$USE_VALIDATOR_NAME_AS_ID" = "true" ]; then
		validator_id="$validator_name"
	fi

    if [ -n "$LOG_DIR" ]; then
        mkdir -p "$LOG_DIR"
    fi

    local uvicorn_bin="${UVICORN_BIN:-uvicorn}"

    if [ -n "$LOG_DIR" ]; then
        env \
            LLM_PROVIDER="${provider}" \
            "${model_env_var}=${model_value}" \
            VALIDATOR_ID="${validator_id}" \
            VALIDATOR_NAME="${validator_name}" \
            AGENT_PROVIDER="${agent_provider}" \
            AGENT_MODEL="${agent_model}" \
            AGENT_ENDPOINT="http://127.0.0.1:${port}" \
            EXPERIMENT_DIR="${EXPERIMENT_DIR}" \
            MOCK_PREPROCESSING_LABEL="${mock_label}" \
            MOCK_PREPROCESSING_ANSWER="${mock_answer}" \
            MOCK_JUDGE_CORRECT_ANSWERS="${mock_judge_correct_answers}" \
            BYZANTINE_MR3_SYNTHESIS="${byzantine_mr3_synthesis}" \
            "${uvicorn_bin}" app:app \
                --host 127.0.0.1 \
                --port "${port}" \
                --log-level info \
            >> "${LOG_DIR}/${validator_name}.log" 2>&1 &
    else
        env \
            LLM_PROVIDER="${provider}" \
            "${model_env_var}=${model_value}" \
            VALIDATOR_ID="${validator_id}" \
            VALIDATOR_NAME="${validator_name}" \
            AGENT_PROVIDER="${agent_provider}" \
            AGENT_MODEL="${agent_model}" \
            AGENT_ENDPOINT="http://127.0.0.1:${port}" \
            EXPERIMENT_DIR="${EXPERIMENT_DIR}" \
            MOCK_PREPROCESSING_LABEL="${mock_label}" \
            MOCK_PREPROCESSING_ANSWER="${mock_answer}" \
            MOCK_JUDGE_CORRECT_ANSWERS="${mock_judge_correct_answers}" \
            BYZANTINE_MR3_SYNTHESIS="${byzantine_mr3_synthesis}" \
            "${uvicorn_bin}" app:app \
                --host 127.0.0.1 \
                --port "${port}" \
                --log-level info &
    fi

    local last_pid=$!
    PIDS+=($last_pid)
    if [ "$agent_provider" != "$provider" ] || [ "$agent_model" != "$model_value" ]; then
        echo "Started ${validator_name} (${agent_provider}/${agent_model}; real roles ${provider}/${model_value}) on port ${port} [pid ${last_pid}]"
    else
        echo "Started ${validator_name} (${provider}/${model_value}) on port ${port} [pid ${last_pid}]"
    fi
}

cd "$SCRIPT_DIR"

# ── 10-validator heterogeneous setup ─────────────────────────────────────────
start_agent 8100 openai    OPENAI_MODEL    gpt-5.4-mini      validator-1  gpt-5.4-mini-1
start_agent 8101 openai    OPENAI_MODEL    gpt-5-mini        validator-2  gpt-5-mini
start_agent 8102 openai    OPENAI_MODEL    gpt-5.4-mini      validator-3  gpt-5.4-mini-2
validator_4_name="claude-haiku-4-5-1"
validator_4_mock_label=""
validator_4_mock_answer=""
validator_4_mock_judge_correct_answers=""
validator_4_provider="anthropic"
validator_4_model="claude-haiku-4-5"
if [ -n "$EXPERIMENT_CONFIG" ]; then
    validator_4_name="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-4")["validator_name"])' "$EXPERIMENT_CONFIG")"
    validator_4_mock_label="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-4").get("mock_preprocessing", {}).get("label", ""))' "$EXPERIMENT_CONFIG")"
    validator_4_mock_answer="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-4").get("mock_preprocessing", {}).get("answer", ""))' "$EXPERIMENT_CONFIG")"
    validator_4_mock_judge_correct_answers="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps(next(v for v in c["validators"] if v["validator_id"] == "validator-4").get("mock_judge_correct_answers", [])))' "$EXPERIMENT_CONFIG")"
    validator_4_provider="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-4")["provider"])' "$EXPERIMENT_CONFIG")"
    validator_4_model="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-4")["model"])' "$EXPERIMENT_CONFIG")"
fi
if [ "$validator_4_provider" = "mock" ]; then
    start_agent 8103 mock MOCK_MODEL mocked-agent validator-4 "$validator_4_name" "$validator_4_mock_label" "$validator_4_mock_answer" "$validator_4_provider" "$validator_4_model" "$validator_4_mock_judge_correct_answers"
else
    start_agent 8103 anthropic ANTHROPIC_MODEL claude-haiku-4-5 validator-4 "$validator_4_name" "$validator_4_mock_label" "$validator_4_mock_answer" "$validator_4_provider" "$validator_4_model"
fi
start_agent 8104 anthropic ANTHROPIC_MODEL claude-sonnet-5   validator-5  claude-sonnet-5
start_agent 8105 anthropic ANTHROPIC_MODEL claude-haiku-4-5  validator-6  claude-haiku-4-5-2
validator_7_name="gemini-3.6-flash-1"
validator_7_mock_label=""
validator_7_mock_answer=""
validator_7_mock_judge_correct_answers=""
validator_7_provider="mock"
validator_7_model="mocked-agent"
if [ -n "$EXPERIMENT_CONFIG" ]; then
    validator_7_name="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-7")["validator_name"])' "$EXPERIMENT_CONFIG")"
    validator_7_mock_label="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-7").get("mock_preprocessing", {}).get("label", ""))' "$EXPERIMENT_CONFIG")"
    validator_7_mock_answer="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-7").get("mock_preprocessing", {}).get("answer", ""))' "$EXPERIMENT_CONFIG")"
    validator_7_mock_judge_correct_answers="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps(next(v for v in c["validators"] if v["validator_id"] == "validator-7").get("mock_judge_correct_answers", [])))' "$EXPERIMENT_CONFIG")"
    validator_7_provider="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-7")["provider"])' "$EXPERIMENT_CONFIG")"
    validator_7_model="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-7")["model"])' "$EXPERIMENT_CONFIG")"
fi
if [ "$validator_7_provider" = "mock" ]; then
    start_agent 8106 mock MOCK_MODEL mocked-agent validator-7 "$validator_7_name" "$validator_7_mock_label" "$validator_7_mock_answer" "$validator_7_provider" "$validator_7_model" "$validator_7_mock_judge_correct_answers"
else
    start_agent 8106 gemini GEMINI_MODEL gemini-3.6-flash validator-7 "$validator_7_name" "$validator_7_mock_label" "$validator_7_mock_answer" "$validator_7_provider" "$validator_7_model"
fi
validator_8_name="gemini-3.6-flash-2"
validator_8_mock_label=""
validator_8_mock_answer=""
validator_8_mock_judge_correct_answers=""
validator_8_provider="gemini"
validator_8_model="gemini-3.6-flash"
if [ -n "$EXPERIMENT_CONFIG" ]; then
    validator_8_name="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-8")["validator_name"])' "$EXPERIMENT_CONFIG")"
    validator_8_mock_label="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-8").get("mock_preprocessing", {}).get("label", ""))' "$EXPERIMENT_CONFIG")"
    validator_8_mock_answer="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-8").get("mock_preprocessing", {}).get("answer", ""))' "$EXPERIMENT_CONFIG")"
    validator_8_mock_judge_correct_answers="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps(next(v for v in c["validators"] if v["validator_id"] == "validator-8").get("mock_judge_correct_answers", [])))' "$EXPERIMENT_CONFIG")"
    validator_8_provider="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-8")["provider"])' "$EXPERIMENT_CONFIG")"
    validator_8_model="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-8")["model"])' "$EXPERIMENT_CONFIG")"
fi
if [ "$validator_8_provider" = "mock" ]; then
    start_agent 8107 mock MOCK_MODEL mocked-agent validator-8 "$validator_8_name" "$validator_8_mock_label" "$validator_8_mock_answer" "$validator_8_provider" "$validator_8_model" "$validator_8_mock_judge_correct_answers"
else
    start_agent 8107 gemini GEMINI_MODEL gemini-3.6-flash validator-8 "$validator_8_name" "$validator_8_mock_label" "$validator_8_mock_answer" "$validator_8_provider" "$validator_8_model"
fi
start_agent 8108 deepseek  DEEPSEEK_MODEL  deepseek-v4-flash validator-9  deepseek-v4-flash
validator_10_name="deepseek-v4-pro"
validator_10_mock_label=""
validator_10_mock_answer=""
validator_10_mock_judge_correct_answers=""
validator_10_byzantine_mr3_synthesis="false"
validator_10_provider="deepseek"
validator_10_model="deepseek-v4-pro"
if [ -n "$EXPERIMENT_CONFIG" ]; then
    validator_10_name="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-10")["validator_name"])' "$EXPERIMENT_CONFIG")"
    validator_10_mock_label="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-10").get("mock_preprocessing", {}).get("label", ""))' "$EXPERIMENT_CONFIG")"
    validator_10_mock_answer="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-10").get("mock_preprocessing", {}).get("answer", ""))' "$EXPERIMENT_CONFIG")"
    validator_10_mock_judge_correct_answers="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(json.dumps(next(v for v in c["validators"] if v["validator_id"] == "validator-10").get("mock_judge_correct_answers", [])))' "$EXPERIMENT_CONFIG")"
    validator_10_byzantine_mr3_synthesis="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(str(next(v for v in c["validators"] if v["validator_id"] == "validator-10").get("byzantine_mr3_synthesis", False)).lower())' "$EXPERIMENT_CONFIG")"
    validator_10_provider="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-10")["provider"])' "$EXPERIMENT_CONFIG")"
    validator_10_model="$(python3 -c 'import json,sys; c=json.load(open(sys.argv[1])); print(next(v for v in c["validators"] if v["validator_id"] == "validator-10")["model"])' "$EXPERIMENT_CONFIG")"
fi
if [ "$validator_10_provider" = "mock" ]; then
    start_agent 8109 mock MOCK_MODEL mocked-agent validator-10 "$validator_10_name" "$validator_10_mock_label" "$validator_10_mock_answer" "$validator_10_provider" "$validator_10_model" "$validator_10_mock_judge_correct_answers" "$validator_10_byzantine_mr3_synthesis"
else
    start_agent 8109 deepseek DEEPSEEK_MODEL deepseek-v4-pro validator-10 "$validator_10_name" "$validator_10_mock_label" "$validator_10_mock_answer" "$validator_10_provider" "$validator_10_model" "$validator_10_mock_judge_correct_answers" "$validator_10_byzantine_mr3_synthesis"
fi

echo ""
echo "All 10 agent servers started. Waiting (Ctrl-C to stop)..."
wait
