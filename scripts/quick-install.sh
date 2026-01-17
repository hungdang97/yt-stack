#!/bin/bash

# YouTube Downloader Stack - Quick Install Script
# Tự động cài đặt Docker, clone repo, chuẩn bị môi trường

set -e

echo "========================================="
echo "YouTube Downloader Stack - Quick Install"
echo "========================================="
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Functions
print_success() {
    echo -e "${GREEN}✓${NC} $1"
}

print_error() {
    echo -e "${RED}✗${NC} $1"
}

print_info() {
    echo -e "${YELLOW}ℹ${NC} $1"
}

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    print_error "Please run as root (use sudo)"
    exit 1
fi

print_info "Starting installation..."
echo ""

# Step 1: Update system
print_info "Step 1/6: Updating system packages..."
apt update -qq
apt upgrade -y -qq
print_success "System updated"
echo ""

# Step 2: Install Docker
print_info "Step 2/6: Installing Docker..."
if command -v docker &> /dev/null; then
    print_success "Docker already installed: $(docker --version)"
else
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh > /dev/null 2>&1
    rm get-docker.sh
    print_success "Docker installed: $(docker --version)"
fi
echo ""

# Step 3: Install Docker Compose Plugin
print_info "Step 3/6: Installing Docker Compose..."
if docker compose version &> /dev/null; then
    print_success "Docker Compose already installed: $(docker compose version)"
else
    apt install -y docker-compose-plugin -qq
    print_success "Docker Compose installed: $(docker compose version)"
fi
echo ""

# Step 4: Configure Firewall
print_info "Step 4/6: Configuring firewall..."
ufw --force enable
ufw allow 22/tcp comment 'SSH' > /dev/null 2>&1
ufw allow 80/tcp comment 'HTTP' > /dev/null 2>&1
ufw allow 443/tcp comment 'HTTPS' > /dev/null 2>&1
ufw allow 1111/tcp comment 'WARP Proxy' > /dev/null 2>&1
ufw allow 2222/tcp comment 'Direct Proxy' > /dev/null 2>&1
print_success "Firewall configured (ports 22, 80, 443, 1111, 2222)"
echo ""

# Step 5: Clone Repository
print_info "Step 5/6: Cloning repository..."
if [ -d "/opt/yt-stack" ]; then
    print_info "Directory /opt/yt-stack already exists, updating..."
    cd /opt/yt-stack
    git pull origin main > /dev/null 2>&1
    print_success "Repository updated"
else
    cd /opt
    git clone https://github.com/BlueByteVietNam/yt-stack.git > /dev/null 2>&1
    cd yt-stack
    print_success "Repository cloned to /opt/yt-stack"
fi
echo ""

# Step 6: Setup .env file
print_info "Step 6/6: Setting up environment file..."
if [ ! -f ".env" ]; then
    cp .env.example .env
    print_success "Created .env file from template"
else
    print_success ".env file already exists"
fi
echo ""

# Done
echo "========================================="
echo -e "${GREEN}Installation Complete!${NC}"
echo "========================================="
echo ""
echo "Next steps:"
echo ""
echo "1. Configure your domain:"
echo "   ${YELLOW}cd /opt/yt-stack${NC}"
echo "   ${YELLOW}nano .env${NC}"
echo "   Edit DOMAIN and EMAIL"
echo ""
echo "2. Build and start services:"
echo "   ${YELLOW}docker compose build --no-cache${NC}"
echo "   ${YELLOW}docker compose up -d${NC}"
echo ""
echo "3. Check logs:"
echo "   ${YELLOW}docker compose logs -f${NC}"
echo ""
echo "4. Test deployment:"
echo "   ${YELLOW}curl https://your-domain.com/health${NC}"
echo ""
echo "See DEPLOYMENT_GUIDE.md for detailed instructions."
echo ""
