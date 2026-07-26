#!/bin/bash
# Run on moa-chain-0. Starts the agent server on every cluster machine.
# Reads machine list and temperatures from configs/cluster.json.
#
# Usage:
#   cd ~/moa-chain && bash scripts/start-cluster.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster.json"

MODEL=$(python3 -c "import json; print(json.load(open('$CONFIG'))['model'])")
PORT=$(python3 -c "import json; print(json.load(open('$CONFIG'))['port'])")

echo "=========================================="
echo " Starting cluster agents"
echo " Model : $MODEL"
echo " Port  : $PORT"
echo "=========================================="
echo ""

start_agent() {
    local machine=$1
    local temp=$2

    # Heredoc avoids quoting conflicts between bash and the JSON in the warmup curl.
    # Variables expanded here ($MODEL, $temp, $PORT) are substituted before sending to remote.
    # Remote-side variables ($secs etc.) are escaped with \ to prevent local expansion.
    ssh "$machine" bash -s 2>&1 << EOF | sed "s/^/[$machine] /"
        # (Re)start Ollama. Always restart so any env var changes take effect.
        pkill ollama 2>/dev/null || true
        sleep 2
        nohup ollama serve > /tmp/ollama.log 2>&1 &
        disown
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
        echo 'Warming up model...'
        curl -sf --max-time 90 http://127.0.0.1:11434/api/generate \
            -d '{"model":"$MODEL","prompt":"hi","stream":false}' > /dev/null \
            && echo 'Warmup done' || echo 'Warmup failed (model may load on first real call)'

        # Kill any existing agent on this port
        pkill -f 'uvicorn app:app' 2>/dev/null || true
        sleep 1

        # Start the agent server
        cd ~/agent-python
        export OLLAMA_MODEL=$MODEL
        export LLM_TEMPERATURE=$temp
        export LLM_TIMEOUT_SECONDS=300
        export LABEL_MAX_CONCURRENCY=2
        export ANSWER_MAX_CONCURRENCY=2
        export JUDGE_MAX_CONCURRENCY=2
        nohup .venv/bin/uvicorn app:app --host 0.0.0.0 --port $PORT > /tmp/agent.log 2>&1 &
        disown
        echo 'Agent started (temperature=$temp)'
EOF
}

# Start all agents in parallel
while IFS='|' read -r machine url temp; do
    start_agent "$machine" "$temp" &
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}\")
")
wait

echo ""
echo "All agents started. Waiting for health checks..."
echo ""

# Poll /health on each agent until reachable or timeout
ALL_OK=true
while IFS='|' read -r machine url temp; do
    seconds=0
    until curl -sf "$url/health" 2>/dev/null | grep -q '"reachable":true'; do
        seconds=$((seconds + 1))
        if [ "$seconds" -ge 120 ]; then
            echo "[$machine] TIMEOUT after 120s — check logs: ssh $machine 'cat /tmp/agent.log'"
            ALL_OK=false
            break
        fi
        sleep 2
    done
    if [ "$seconds" -lt 120 ]; then
        echo "[$machine] healthy (temperature=$temp)"
    fi
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}\")
")

echo ""
if [ "$ALL_OK" = true ]; then
    echo "All agents are healthy."
else
    echo "Some agents failed to start. Check logs above."
    exit 1
fi
