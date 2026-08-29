#!/usr/bin/env bash
# Polls /health on all 10 local agents and prints a status table.
# Exits with status 1 if any agent is down or unhealthy.
# Run before integration tests to confirm the cluster is ready.
#
# Usage:
#   cd ~/moa-chain && bash scripts/check-local-agents.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="$PROJECT_DIR/configs/cluster-local-openai.json"

echo "=========================================="
echo " Local agent health check"
echo "=========================================="
printf "\n%-16s %-30s %-14s %-6s %-10s %-10s %s\n" \
    "VALIDATOR" "URL" "MODEL" "TEMP" "AGENT" "PROVIDER" "REACHABLE"
printf "%-16s %-30s %-14s %-6s %-10s %-10s %s\n" \
    "---------" "---" "-----" "----" "-----" "--------" "---------"

ALL_OK=true

check_agent() {
    local machine=$1 url=$2 temp=$3 model=$4

    local response
    response=$(curl -sf --max-time 5 "$url/health" 2>/dev/null || true)

    local agent_status provider_name reachable_str actual_model

    if [ -z "$response" ]; then
        agent_status="DOWN"
        provider_name="-"
        reachable_str="no"
        printf "%-16s %-30s %-14s %-6s %-10s %-10s %s\n" \
            "$machine" "$url" "$model" "$temp" "$agent_status" "$provider_name" "$reachable_str"
        return 1
    fi

    agent_status=$(python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
print(d.get('status', 'error'))
" <<< "$response" 2>/dev/null || echo "error")

    provider_name=$(python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
print(d.get('provider', '-'))
" <<< "$response" 2>/dev/null || echo "-")

    reachable_str=$(python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
print('yes' if d.get('reachable') else 'no')
" <<< "$response" 2>/dev/null || echo "no")

    actual_model=$(python3 -c "
import json, sys
d = json.loads(sys.stdin.read())
print(d.get('model', '-'))
" <<< "$response" 2>/dev/null || echo "-")

    if [ "$actual_model" != "$model" ] && [ "$actual_model" != "-" ]; then
        agent_status="MODEL_MISMATCH"
    fi

    printf "%-16s %-30s %-14s %-6s %-10s %-10s %s\n" \
        "$machine" "$url" "$model" "$temp" "$agent_status" "$provider_name" "$reachable_str"

    if [ "$agent_status" != "ok" ] || [ "$reachable_str" != "yes" ]; then
        return 1
    fi
}

# Run checks sequentially (10 agents, fast /health call each).
set +e
for entry in $(python3 -c "
import json
for a in json.load(open('$CONFIG'))['agents']:
    print(f\"{a['machine']}|{a['url']}|{a['temperature']}|{a['model']}\")
"); do
    IFS='|' read -r machine url temp model <<< "$entry"
    if ! check_agent "$machine" "$url" "$temp" "$model"; then
        ALL_OK=false
    fi
done
set -e

echo ""
if [ "$ALL_OK" = true ]; then
    echo "All 10 local agents healthy."
else
    echo "Some agents are not ready."
    echo "  Start them : bash scripts/start-local-agents.sh"
    echo "  View logs  : logs/local-agents/<validator>.log"
    exit 1
fi
