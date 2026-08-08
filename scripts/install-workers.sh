#!/bin/bash
# Run on moa-chain-0.
# Deploys agent-python, installs Ollama, and pulls each machine's model.
# Each agent now runs a different model; model is specified per-agent in cluster.json.
#
# Usage:
#   cd ~/moa-chain && bash scripts/install-workers.sh
#
# Safe to re-run: skips steps that are already done.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster.json"
AGENT_SRC="$PROJECT_DIR/agent-python"
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3)
RSYNC_RSH="ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3"

# Read machine list from cluster.json using Python (always available).
MACHINES=$(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(a['machine'])
")

echo "=========================================="
echo " MoA-Chain cluster setup"
echo " Machines: $(echo $MACHINES | tr '\n' ' ')"
echo "=========================================="
echo ""

# ── Step 1a: deploy agent-python and install Python deps ──────────────────────

deploy_agent() {
    local machine=$1

    echo "[$machine] Deploying agent-python..."
    rsync -az --delete \
        -e "$RSYNC_RSH" \
        --exclude='.venv' \
        --exclude='__pycache__' \
        --exclude='*.egg-info' \
        --exclude='*.pyc' \
        --exclude='.pytest_cache' \
        --exclude='.idea' \
        "$AGENT_SRC/" "$machine:~/agent-python/" \
        2>&1 | sed "s/^/[$machine] /"

    echo "[$machine] Installing Python dependencies..."
    ssh "${SSH_OPTS[@]}" "$machine" '
        cd ~/agent-python
        if [ ! -x .venv/bin/python3 ] && ! python3 -m venv .venv; then
            sudo apt install -y python3.12-venv -q
            python3 -m venv .venv
        fi
        .venv/bin/pip install . -q
        echo "deps ok"
    ' 2>&1 | sed "s/^/[$machine] /"

    echo "[$machine] agent-python ready."
}

echo "--- Step 1a: deploying agent-python (parallel) ---"
for machine in $MACHINES; do
    deploy_agent "$machine" &
done
wait
echo ""

# ── Step 1b: install Ollama ───────────────────────────────────────────────────

install_ollama() {
    local machine=$1

    ssh "${SSH_OPTS[@]}" "$machine" '
        if command -v ollama &>/dev/null; then
            echo "ollama already installed: $(ollama --version 2>/dev/null)"
        else
            echo "Installing ollama..."
            curl -fsSL https://ollama.ai/install.sh | sh
            echo "ollama installed: $(ollama --version 2>/dev/null)"
        fi
    ' 2>&1 | sed "s/^/[$machine] /"
}

echo "--- Step 1b: installing Ollama (parallel) ---"
for machine in $MACHINES; do
    install_ollama "$machine" &
done
wait
echo ""

# ── Step 1c: pull the per-machine model ──────────────────────────────────────
# Each machine runs a different model; pull only that machine's model.
# Each machine starts Ollama temporarily if it is not already running,
# pulls the model, then leaves Ollama running (start-cluster.sh will manage it).

pull_model() {
    local machine=$1
    local model=$2

    ssh "${SSH_OPTS[@]}" "$machine" "
        # Start ollama if not already running
        if ! curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; then
            echo 'Starting ollama for model pull...'
            nohup ollama serve > /tmp/ollama-setup.log 2>&1 &
            # Wait up to 15s for ollama to be ready
            seconds=0
            until curl -sf http://127.0.0.1:11434 > /dev/null 2>&1; do
                seconds=\$((seconds + 1))
                if [ \$seconds -ge 15 ]; then
                    echo 'ERROR: ollama did not start in time'
                    exit 1
                fi
                sleep 1
            done
        fi

        if ollama list | awk 'NR > 1 {print \$1}' | grep -Fxq '$model'; then
            echo '$model already present, skipping pull'
        else
            echo 'Pulling $model (this may take several minutes)...'
            ollama pull $model
            echo 'Pull complete'
        fi
    " 2>&1 | sed "s/^/[$machine] /"
}

echo "--- Step 1c: pulling per-machine models in parallel ---"
echo "    (this takes 5-10 min per machine on first run)"
echo ""
while IFS='|' read -r machine model; do
    pull_model "$machine" "$model" &
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['model']}\")
")
wait
echo ""

# ── Checkpoints ───────────────────────────────────────────────────────────────

echo "=========================================="
echo " Setup complete. Verify with:"
echo "=========================================="
while IFS='|' read -r machine model; do
    echo "  ssh $machine 'ollama list | grep $model && ~/agent-python/.venv/bin/python3 -c \"import fastapi; print(fastapi.__version__)\"'"
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['model']}\")
")
