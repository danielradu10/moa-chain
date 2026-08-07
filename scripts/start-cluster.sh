#!/bin/bash
# Run on moa-chain-0. Starts the agent server on every cluster machine.
# Reads machine list, per-agent models, and temperatures from configs/cluster.json.
#
# Usage:
#   cd ~/moa-chain && bash scripts/start-cluster.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster.json"

LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-300}"
LLM_NUM_CTX="${LLM_NUM_CTX:-4096}"
LLM_NUM_PREDICT="${LLM_NUM_PREDICT:-256}"
LLM_THINK="${LLM_THINK:-false}"
JUDGE_MAX_CONCURRENCY="${JUDGE_MAX_CONCURRENCY:-2}"
MODEL_WARMUP_TIMEOUT="${MODEL_WARMUP_TIMEOUT:-240}"
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3)

PORT=$(python3 -c "import json; print(json.load(open('$CONFIG'))['port'])")

echo "=========================================="
echo " Starting cluster agents"
echo " Port  : $PORT"
echo "=========================================="
echo ""

start_agent() {
    local machine=$1
    local temp=$2
    local model=$3

    # Heredoc avoids quoting conflicts between bash and the JSON in the warmup curl.
    # Variables expanded here ($model, $temp, $PORT) are substituted before sending to remote.
    # Remote-side variables ($secs etc.) are escaped with \ to prevent local expansion.
    ssh "${SSH_OPTS[@]}" "$machine" bash -s 2>&1 << EOF | sed "s/^/[$machine] /"
        # Keep Ollama and its model cache alive across trials. Start it only
        # when it is not already reachable.
        if ! curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; then
            nohup ollama serve > /tmp/ollama.log 2>&1 &
            disown
        fi
        secs=0
        until curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; do
            secs=\$((secs + 1))
            if [ \$secs -ge 30 ]; then
                echo 'ERROR: Ollama did not start after 30s'
                exit 1
            fi
            sleep 1
        done
        echo 'Ollama ready'

        # Warm up: force the model into memory before serving requests.
        # The first inference call loads the model (~10-30s); subsequent calls are fast.
        if ollama ps | awk 'NR > 1 {print \$1}' | grep -Fxq '$model'; then
            echo 'Model $model already loaded; skipping warmup'
        else
            echo 'Warming up model $model...'
            curl -sf --max-time $MODEL_WARMUP_TIMEOUT -H 'Content-Type: application/json' http://127.0.0.1:11434/api/generate \
                -d '{"model":"$model","prompt":"hi","stream":false,"think":false,"options":{"num_ctx":$LLM_NUM_CTX,"num_predict":1}}' > /dev/null \
                && echo 'Warmup done' || { echo 'ERROR: model warmup failed'; exit 1; }
        fi

        # Kill any existing agent on this port
        pkill -f 'uvicorn app:app' 2>/dev/null || true
        sleep 1

        # Start the agent server
        cd ~/agent-python
        export OLLAMA_MODEL=$model
        export LLM_TEMPERATURE=$temp
        export LLM_TIMEOUT_SECONDS=$LLM_TIMEOUT_SECONDS
        export LLM_NUM_CTX=$LLM_NUM_CTX
        export LLM_NUM_PREDICT=$LLM_NUM_PREDICT
        export LLM_THINK=$LLM_THINK
        export LABEL_MAX_CONCURRENCY=2
        export ANSWER_MAX_CONCURRENCY=2
        export JUDGE_MAX_CONCURRENCY=$JUDGE_MAX_CONCURRENCY
        nohup .venv/bin/uvicorn app:app --host 0.0.0.0 --port $PORT > /tmp/agent.log 2>&1 &
        disown
        echo 'Agent started (model=$model, temperature=$temp, num_ctx=$LLM_NUM_CTX, num_predict=$LLM_NUM_PREDICT, think=$LLM_THINK)'
EOF
}

# Start all agents in parallel
while IFS='|' read -r machine url temp model; do
    start_agent "$machine" "$temp" "$model" &
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}|{a['model']}\")
")
wait

echo ""
echo "All agents started. Waiting for health checks..."
echo ""

# Poll /health on all agents in parallel with a real wall-clock deadline.
check_agent_health() {
    local machine=$1
    local url=$2
    local temp=$3
    local model=$4
    local deadline=$((SECONDS + 120))

    until curl -sf --max-time 5 "$url/health" 2>/dev/null | grep -q '"reachable":true'; do
        if [ "$SECONDS" -ge "$deadline" ]; then
            echo "[$machine] TIMEOUT after 120s — check logs: ssh $machine 'cat /tmp/agent.log'"
            return 1
        fi
        sleep 2
    done
    echo "[$machine] healthy (model=$model, temperature=$temp)"
}

ALL_OK=true
pids=()
while IFS='|' read -r machine url temp model; do
    check_agent_health "$machine" "$url" "$temp" "$model" &
    pids+=("$!")
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}|{a['model']}\")
")
for pid in "${pids[@]}"; do
    if ! wait "$pid"; then
        ALL_OK=false
    fi
done

echo ""
if [ "$ALL_OK" = true ]; then
    echo "All agents are healthy."
else
    echo "Some agents failed to start. Check logs above."
    exit 1
fi
