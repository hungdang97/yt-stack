#!/bin/bash
# Agent self-update script
# Called by /control/update-agent endpoint

echo "=== VPS Agent Self-Update ==="

PROJECT_DIR="/opt/yt-stack"
AGENT_DIR="$PROJECT_DIR/vps-agent"

# 1. Pull latest code
echo "[1/3] Pulling latest code..."
cd $PROJECT_DIR
git fetch origin main
git reset --hard origin/main

# 2. Rebuild agent binary
echo "[2/3] Rebuilding agent..."
cd $AGENT_DIR
if ! /usr/local/go/bin/go build -o vps-agent . 2>&1; then
    echo "ERROR: go build failed"
    exit 1
fi
echo "✓ Binary rebuilt: $(ls -la vps-agent | awk '{print $6, $7, $8, $9}')"

# 3. Restart agent service
echo "[3/3] Restarting agent service..."
systemctl restart vps-agent

echo "✓ Agent update complete!"
