#!/bin/bash
# Pulls test results and logs from moa-chain-0 back to the local repository.
# Run from your local machine after a distributed test run.
#
# Collects:
#   testresults/            — JSON summaries and tee'd test output
#   integrationtests/logs/  — per-validator slog traces
#   testresults/agent-logs/ — /tmp/agent.log from each cluster machine
#
# Usage:
#   bash scripts/collect-logs.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE="$(cd "$PROJECT_DIR/../.." && pwd)"

MASTER="${MOA_MASTER_HOST:?MOA_MASTER_HOST is not set}"
KEY="${MOA_MASTER_KEY:?MOA_MASTER_KEY is not set}"
SSH_CMD="ssh -i $KEY -l ubuntu"
REMOTE_DIR="~/moa-chain"
CONFIG="$REMOTE_DIR/configs/cluster.json"

echo "=========================================="
echo " Collecting logs from moa-chain-0"
echo "=========================================="

# Step 1: gather /tmp/agent.log from every cluster machine into
# testresults/agent-logs/ on moa-chain-0, using moa-chain-0 as a hop.
echo ""
echo "Collecting agent logs from cluster machines..."
$SSH_CMD "$MASTER" bash -s << EOF
    set -uo pipefail
    mkdir -p $REMOTE_DIR/testresults/agent-logs
    for machine in \$(python3 -c "import json,os; [print(a['machine']) for a in json.load(open(os.path.expanduser('$CONFIG')))['agents']]"); do
        dest=$REMOTE_DIR/testresults/agent-logs/\${machine}-agent.log
        if ssh "\$machine" "test -f /tmp/agent.log" 2>/dev/null; then
            ssh "\$machine" "cat /tmp/agent.log" > "\$dest"
            echo "  [\$machine] agent.log collected"
        else
            echo "  [\$machine] agent.log not found"
        fi
    done
EOF

# Step 2: pull testresults/ (JSON summaries, tee'd test output, agent logs)
echo ""
echo "Syncing testresults/ ..."
rsync -avz \
    -e "$SSH_CMD" \
    "ubuntu@$MASTER:$REMOTE_DIR/testresults/" \
    "$PROJECT_DIR/testresults/"

# Step 3: pull per-validator slog traces
echo ""
echo "Syncing integrationtests/logs/ ..."
rsync -avz \
    -e "$SSH_CMD" \
    "ubuntu@$MASTER:$REMOTE_DIR/integrationtests/logs/" \
    "$PROJECT_DIR/integrationtests/logs/"

echo ""
echo "Done. Collected:"
echo "  testresults/             — JSON summaries + test output"
echo "  testresults/agent-logs/  — /tmp/agent.log from each machine"
echo "  integrationtests/logs/   — per-validator slog traces"
