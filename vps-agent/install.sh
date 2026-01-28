#!/bin/bash
set -e

echo "=== YT-Stack VPS Auto-Setup ==="

# Check root permission
if [ "$(id -u)" != "0" ]; then
   echo "❌ Error: This script must be run as root."
   echo "👉 Please run: curl -sSL ... | sudo HUB_URL=... bash"
   exit 1
fi

# Required ENV
HUB_URL="${HUB_URL:?HUB_URL is required}"

# Fixed defaults
PROJECT_DIR="/opt/yt-stack"
AGENT_PORT="9000"
GIT_REPO="https://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@github.com/BlueByteVietNam/yt-stack.git"
GIT_BRANCH="main"

echo "Configuration:"
echo "  Hub URL: $HUB_URL"
echo "  Project Dir: $PROJECT_DIR"
echo "  Agent Port: $AGENT_PORT"
echo ""
echo "Agent will auto-generate all configuration..."
echo ""

# 1. Install dependencies
echo "[1/5] Installing dependencies..."
apt-get update -qq
apt-get install -y docker.io docker-compose git curl jq

# 2. Clone project
echo "[2/5] Cloning project..."
if [ ! -d "$PROJECT_DIR" ]; then
    git clone -b $GIT_BRANCH $GIT_REPO $PROJECT_DIR
else
    echo "Project exists, pulling latest..."
    cd $PROJECT_DIR && git pull origin $GIT_BRANCH
fi

# 3. Clean install - Reset existing service
echo "[3/5] Cleaning up old service..."
if [ -d "$PROJECT_DIR" ]; then
    echo "  Stopping existing Docker containers..."
    cd $PROJECT_DIR && docker-compose down --remove-orphans || true
fi

if systemctl list-units --full -all | grep -Fq 'vps-agent.service'; then
    echo "  Stopping and removing existing vps-agent service..."
    systemctl stop vps-agent || true
    systemctl disable vps-agent || true
    rm -f /etc/systemd/system/vps-agent.service
    systemctl daemon-reload
fi

# 4. Download Agent binary
echo "[4/5] Installing VPS Agent..."
AGENT_DIR="$PROJECT_DIR/vps-agent"
mkdir -p $AGENT_DIR
curl -sSL $HUB_URL/downloads/vps-agent -o $AGENT_DIR/vps-agent
chmod +x $AGENT_DIR/vps-agent

# 4. Create Agent config
cat > $AGENT_DIR/agent.env <<EOF
HUB_URL=$HUB_URL
PROJECT_DIR=$PROJECT_DIR
AGENT_PORT=$AGENT_PORT
EOF

# 5. Create systemd service
echo "[4/5] Creating systemd service..."
cat > /etc/systemd/system/vps-agent.service <<EOF
[Unit]
Description=VPS Agent for YT-Stack
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$AGENT_DIR
EnvironmentFile=$AGENT_DIR/agent.env
ExecStart=$AGENT_DIR/vps-agent
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# 6. Start Agent
echo "[5/5] Starting Agent..."
systemctl daemon-reload
systemctl enable vps-agent
systemctl restart vps-agent

echo ""
echo "✓ Setup complete!"
echo "✓ Agent is auto-generating configuration and registering with Hub"
echo "✓ Service will be deployed automatically"
echo ""
echo "Monitor progress:"
echo "  journalctl -u vps-agent -f"
echo ""
echo "Check service:"
echo "  cd $PROJECT_DIR && docker-compose ps"
