#!/bin/bash
# Run on moa-chain-0. Polls /health on every agent and prints a status table.
# Useful to run before integration tests to confirm all agents are up.
#
# Usage:
#   cd ~/moa-chain && bash scripts/health-check.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster.json"

echo "=========================================="
echo " Cluster health check"
echo "=========================================="
printf "\n%-14s %-30s %-15s %-6s %-8s %s\n" "MACHINE" "URL" "MODEL" "TEMP" "AGENT" "OLLAMA"
printf "%-14s %-30s %-15s %-6s %-8s %s\n" "-------" "---" "-----" "----" "-----" "------"

ALL_OK=true

while IFS='|' read -r machine url temp model; do
    response=$(curl -sf --max-time 5 "$url/health" 2>/dev/null || true)

    if [ -z "$response" ]; then
        agent_status="DOWN"
        ollama_status="unknown"
        ALL_OK=false
    else
        result=$(python3 -c "
import json, sys
try:
    d = json.loads(sys.stdin.read())
    reachable = 'yes' if d.get('reachable') else 'no'
    status = d.get('status', '?')
    print(f'{status}|{reachable}')
except Exception as e:
    print(f'error|error')
" <<< "$response")

        IFS='|' read -r agent_status ollama_status <<< "$result"

        if [ "$agent_status" != "ok" ] || [ "$ollama_status" != "yes" ]; then
            ALL_OK=false
        fi
    fi

    actual_model=$(python3 -c "import json,sys; print(json.loads(sys.stdin.read()).get('model',''))" <<< "${response:-{}}" 2>/dev/null || true)
    if [ -n "$response" ] && [ "$actual_model" != "$model" ]; then
        agent_status="MODEL_MISMATCH"
        ALL_OK=false
    fi
    printf "%-14s %-30s %-15s %-6s %-8s %s\n" "$machine" "$url" "$model" "$temp" "$agent_status" "$ollama_status"
done < <(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}|{a['model']}\")
")

echo ""
if [ "$ALL_OK" = true ]; then
    echo "All 10 agents healthy."
else
    echo "Some agents are not ready. Run 'bash scripts/start-cluster.sh' or check logs:"
    echo "  ssh <machine> 'cat /tmp/agent.log'"
    exit 1
fi
