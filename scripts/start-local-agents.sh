#!/usr/bin/env bash
# Starts 10 agent-python FastAPI processes on ports 8100-8109, one per validator,
# using the OpenAI provider. Each process is configured from
# configs/cluster-local-openai.json. OPENAI_API_KEY must be set in the
# calling environment — it is never written to disk.
#
# Usage:
#   cd ~/moa-chain && bash scripts/start-local-agents.sh
#
# Optional overrides (environment variables):
#   LLM_TIMEOUT_SECONDS=120
#   LABEL_MAX_CONCURRENCY=2
#   ANSWER_MAX_CONCURRENCY=2
#   JUDGE_MAX_CONCURRENCY=2
#   HEALTH_TIMEOUT=90

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster-local-openai.json"
AGENT_DIR="$PROJECT_DIR/agent-python"
LOG_DIR="$PROJECT_DIR/logs/local-agents"
PID_DIR="$LOG_DIR/pids"

LLM_TIMEOUT_SECONDS="${LLM_TIMEOUT_SECONDS:-120}"
LABEL_MAX_CONCURRENCY="${LABEL_MAX_CONCURRENCY:-4}"
ANSWER_MAX_CONCURRENCY="${ANSWER_MAX_CONCURRENCY:-4}"
JUDGE_MAX_CONCURRENCY="${JUDGE_MAX_CONCURRENCY:-4}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-60}"

# ── Pre-flight checks ──────────────────────────────────────────────────────────

if [ -z "${OPENAI_API_KEY:-}" ]; then
    echo "ERROR: OPENAI_API_KEY is not set."
    echo "Export it before running:"
    echo "  export OPENAI_API_KEY=sk-..."
    exit 1
fi

if [ ! -f "$CONFIG" ]; then
    echo "ERROR: Config not found: $CONFIG"
    exit 1
fi

if [ ! -f "$AGENT_DIR/.venv/bin/uvicorn" ]; then
    echo "ERROR: Python venv not found at agent-python/.venv. Run:"
    echo "  cd agent-python && make install"
    exit 1
fi

# ── Directory setup ────────────────────────────────────────────────────────────

mkdir -p "$LOG_DIR" "$PID_DIR"

# ── Helper: print one pipe-separated line per agent ───────────────────────────
# Format: machine|url|temperature|model
parse_agents() {
    python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}|{a['model']}\")
"
}

# ── Kill any stale processes on ports 8100-8109 ───────────────────────────────

echo "=========================================="
echo " Starting local agent processes"
echo "=========================================="
echo ""
echo "Clearing any stale processes on ports 8100-8109..."
for port in $(seq 8100 8109); do
    lsof -ti tcp:"$port" 2>/dev/null | xargs kill -9 2>/dev/null || true
done
sleep 0.5

# ── Track all launched PIDs for cleanup on failure ────────────────────────────

ALL_PIDS=()

cleanup() {
    if [ "${#ALL_PIDS[@]}" -gt 0 ]; then
        echo ""
        echo "Stopping all started agents..."
        for pid in "${ALL_PIDS[@]}"; do
            kill "$pid" 2>/dev/null || true
        done
        rm -f "$PID_DIR"/*.pid
    fi
}

# ── Start each agent ──────────────────────────────────────────────────────────

start_agent() {
    local machine=$1 url=$2 temp=$3 model=$4

    # Extract port from URL — format is always http://127.0.0.1:PORT
    local port="${url##*:}"

    local pid_file="$PID_DIR/${machine}.pid"
    local log_file="$LOG_DIR/${machine}.log"

    printf "  %-14s port=%-5s model=%-14s temp=%s\n" "$machine" "$port" "$model" "$temp"

    LLM_PROVIDER=openai \
    OPENAI_API_KEY="$OPENAI_API_KEY" \
    OPENAI_MODEL="$model" \
    LLM_TEMPERATURE="$temp" \
    LLM_TIMEOUT_SECONDS="$LLM_TIMEOUT_SECONDS" \
    LABEL_MAX_CONCURRENCY="$LABEL_MAX_CONCURRENCY" \
    ANSWER_MAX_CONCURRENCY="$ANSWER_MAX_CONCURRENCY" \
    JUDGE_MAX_CONCURRENCY="$JUDGE_MAX_CONCURRENCY" \
    "$AGENT_DIR/.venv/bin/uvicorn" app:app \
        --app-dir "$AGENT_DIR" \
        --host 127.0.0.1 \
        --port "$port" \
        > "$log_file" 2>&1 &

    local pid=$!
    echo "$pid" > "$pid_file"
    ALL_PIDS+=("$pid")
}

echo ""
while IFS='|' read -r machine url temp model; do
    start_agent "$machine" "$url" "$temp" "$model"
done < <(parse_agents)

echo ""
echo "All 10 processes launched."
echo "Waiting for health checks (timeout ${HEALTH_TIMEOUT}s each)..."
echo ""

# ── Poll health in parallel ────────────────────────────────────────────────────

check_health() {
    local machine=$1 url=$2 model=$3 temp=$4
    local deadline=$((SECONDS + HEALTH_TIMEOUT))

    until curl -sf --max-time 5 "$url/health" 2>/dev/null | python3 -c "
import json, sys
d = json.load(sys.stdin)
sys.exit(0 if d.get('reachable') else 1)
" 2>/dev/null; do
        if [ "$SECONDS" -ge "$deadline" ]; then
            echo "  [$machine] TIMEOUT after ${HEALTH_TIMEOUT}s"
            echo "    Check log: $LOG_DIR/${machine}.log"
            return 1
        fi
        sleep 2
    done
    printf "  %-14s healthy  model=%-14s temp=%s\n" "[$machine]" "$model" "$temp"
}

# Disable set -e for the wait loop so we can collect all failures.
set +e

HEALTH_PIDS=()
while IFS='|' read -r machine url temp model; do
    check_health "$machine" "$url" "$model" "$temp" &
    HEALTH_PIDS+=("$!")
done < <(parse_agents)

ALL_OK=true
for pid in "${HEALTH_PIDS[@]}"; do
    if ! wait "$pid"; then
        ALL_OK=false
    fi
done

set -e

echo ""
if [ "$ALL_OK" = true ]; then
    echo "All 10 local agents are healthy."
    echo ""
    echo "Point integration tests at this cluster:"
    echo "  MOA_CLUSTER_CONFIG=$CONFIG make test-distributed-mr1"
    echo ""
    echo "Stop all agents when done:"
    echo "  bash scripts/stop-local-agents.sh"
else
    echo "One or more agents failed to become healthy."
    cleanup
    exit 1
fi
