#!/bin/bash
# Pushes the repository to moa-chain-0. Run from your local machine workspace root.
# See scripts/README.md for a full explanation of how this works.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
WORKSPACE="$(cd "$PROJECT_DIR/../.." && pwd)"

MASTER="${MOA_MASTER_HOST:?MOA_MASTER_HOST is not set}"
KEY="${MOA_MASTER_KEY:?MOA_MASTER_KEY is not set}"
REMOTE_DIR="~/moa-chain"

echo "Syncing $PROJECT_DIR -> ubuntu@$MASTER:$REMOTE_DIR ..."

rsync -avz --delete \
    -e "ssh -i $KEY -l ubuntu" \
    --exclude='.git' \
    --exclude='scripts/sync-to-master.sh' \
    --exclude='scripts/README.md' \
    --exclude='.venv' \
    --exclude='agent-python/.venv' \
    --exclude='**/__pycache__' \
    --exclude='**/*.egg-info' \
    --exclude='**/.pytest_cache' \
    --exclude='.idea' \
    --exclude='**/.idea' \
    --exclude='*.pdf' \
    --exclude='bin/' \
    --exclude='clustering/.idea' \
    "$PROJECT_DIR/" "ubuntu@$MASTER:$REMOTE_DIR/"

echo ""
echo "Done. Connect to moa-chain-0 and run the install script:"
echo ""
echo "  ssh -i $KEY -l ubuntu $MASTER"
echo "  cd ~/moa-chain && bash scripts/install-workers.sh"
