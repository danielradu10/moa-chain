#!/bin/bash
# Run on moa-chain-0. Stops the agent server on every cluster machine.
# Ollama is left running by default — restarting it reloads the model (~30s per machine).
#
# Usage:
#   cd ~/moa-chain && bash scripts/stop-cluster.sh
#   cd ~/moa-chain && bash scripts/stop-cluster.sh --stop-ollama

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster.json"
SSH_OPTS=(-o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=5 -o ServerAliveCountMax=3)

STOP_OLLAMA=false
if [ "${1:-}" = "--stop-ollama" ]; then
    STOP_OLLAMA=true
fi

MACHINES=$(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(a['machine'])
")

echo "=========================================="
echo " Stopping cluster agents"
if [ "$STOP_OLLAMA" = true ]; then
    echo " (including Ollama)"
fi
echo "=========================================="
echo ""

stop_machine() {
    local machine=$1

    ssh "${SSH_OPTS[@]}" "$machine" "
        if pkill -f 'uvicorn app:app' 2>/dev/null; then
            echo 'agent stopped'
        else
            echo 'no agent running'
        fi

        if [ '$STOP_OLLAMA' = true ]; then
            if pkill ollama 2>/dev/null; then
                echo 'ollama stopped'
            else
                echo 'no ollama running'
            fi
        fi
    " 2>&1 | sed "s/^/[$machine] /"
}

for machine in $MACHINES; do
    stop_machine "$machine" &
done
wait

echo ""
echo "Done."
