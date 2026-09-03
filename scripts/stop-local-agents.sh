#!/usr/bin/env bash
# Stops all local agent processes started by start-local-agents.sh.
# Uses PID files in logs/local-agents/pids/. Tolerates already-stopped processes.
#
# Usage:
#   cd ~/moa-chain && bash scripts/stop-local-agents.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
PID_DIR="$PROJECT_DIR/logs/local-agents/pids"

echo "=========================================="
echo " Stopping local agent processes"
echo "=========================================="
echo ""

if [ ! -d "$PID_DIR" ]; then
    echo "No PID directory found — no agents to stop."
    exit 0
fi

found=0
for pid_file in "$PID_DIR"/*.pid; do
    [ -f "$pid_file" ] || continue
    machine=$(basename "$pid_file" .pid)
    pid=$(cat "$pid_file")
    found=$((found + 1))
    if kill "$pid" 2>/dev/null; then
        printf "  %-14s stopped  (PID %s)\n" "$machine" "$pid"
    else
        printf "  %-14s already stopped (PID %s)\n" "$machine" "$pid"
    fi
    rm -f "$pid_file"
done

if [ "$found" -eq 0 ]; then
    echo "No PID files found — nothing to stop."
fi

echo ""
echo "Done."
