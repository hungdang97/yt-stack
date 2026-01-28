#!/bin/bash
# Standalone firewall setup script for YT-Stack VPS
# Run: sudo bash setup-firewall.sh

set -e

echo "=== YT-Stack Firewall Setup ==="
echo ""

# Check root permission
if [ "$(id -u)" != "0" ]; then
   echo "❌ Error: This script must be run as root."
   echo "👉 Please run: sudo bash setup-firewall.sh"
   exit 1
fi

# Install UFW if not present
if ! command -v ufw &> /dev/null; then
    echo "Installing UFW..."
    apt-get update -qq
    apt-get install -y ufw
fi

# Disable UFW first to avoid connection issues
echo "Disabling firewall temporarily..."
ufw --force disable

# Set default policies
echo "Setting default policies..."
ufw default deny incoming
ufw default allow outgoing

# Allow all required ports
echo ""
echo "Opening required ports:"
echo "  ✓ SSH (22) - Remote access"
ufw allow 22/tcp

echo "  ✓ HTTP (80) - Web traffic"
ufw allow 80/tcp

echo "  ✓ HTTPS (443) - Secure web traffic"
ufw allow 443/tcp

echo "  ✓ GOST Proxy (1111) - Proxy server"
ufw allow 1111/tcp

echo "  ✓ YT-Downloader API (5001) - Main service"
ufw allow 5001/tcp

echo "  ✓ VPS Agent Control (9000) - Agent management"
ufw allow 9000/tcp

# Enable firewall
echo ""
echo "Enabling firewall..."
ufw --force enable

echo ""
echo "✓ Firewall configured successfully!"
echo ""
echo "Current firewall status:"
ufw status numbered

echo ""
echo "=== Summary ==="
echo "Allowed ports:"
echo "  - 22   (SSH)"
echo "  - 80   (HTTP)"
echo "  - 443  (HTTPS)"
echo "  - 1111 (GOST Proxy)"
echo "  - 5001 (YT-Downloader API)"
echo "  - 9000 (VPS Agent Control)"
echo ""
