#!/bin/bash
# Pulls judge benchmark results from moa-chain-0 into the local repository.
# Run from the local machine after a benchmark run.
#
# Usage:
#   bash scripts/collect-benchmark-results.sh
#   bash scripts/collect-benchmark-results.sh qualification_20260806_run01

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

MASTER="${MOA_MASTER_HOST:?MOA_MASTER_HOST is not set}"
KEY="${MOA_MASTER_KEY:?MOA_MASTER_KEY is not set}"
SSH_CMD="ssh -i $KEY -l ubuntu"
REMOTE_ROOT="~/moa-chain/benchmark_results"
LOCAL_ROOT="$PROJECT_DIR/benchmark_results"
RUN_NAME="${1:-}"

if [ "$#" -gt 1 ]; then
    echo "Usage: bash scripts/collect-benchmark-results.sh [run-name]" >&2
    exit 2
fi

if [ -n "$RUN_NAME" ]; then
    if [[ "$RUN_NAME" == */* || "$RUN_NAME" == "." || "$RUN_NAME" == ".." ]]; then
        echo "Invalid run name: $RUN_NAME" >&2
        exit 2
    fi
    REMOTE_SOURCE="$REMOTE_ROOT/$RUN_NAME/"
    LOCAL_DEST="$LOCAL_ROOT/$RUN_NAME/"
    DESCRIPTION="benchmark run $RUN_NAME"
else
    REMOTE_SOURCE="$REMOTE_ROOT/"
    LOCAL_DEST="$LOCAL_ROOT/"
    DESCRIPTION="all benchmark results"
fi

mkdir -p "$LOCAL_DEST"

echo "Pulling $DESCRIPTION from ubuntu@$MASTER ..."
rsync -avz --progress \
    -e "$SSH_CMD" \
    "ubuntu@$MASTER:$REMOTE_SOURCE" \
    "$LOCAL_DEST"

echo ""
echo "Done. Results are in: $LOCAL_DEST"
