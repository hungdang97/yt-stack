#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== VPS Initial Setup ===${NC}"

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Please run as root (use sudo)${NC}"
    exit 1
fi

# Update system
echo -e "${GREEN}Updating system packages...${NC}"
apt-get update
apt-get upgrade -y

# Install Docker
if ! command -v docker &> /dev/null; then
    echo -e "${GREEN}Installing Docker...${NC}"
    curl -fsSL https://get.docker.com -o get-docker.sh
    sh get-docker.sh
    rm get-docker.sh

    # Start and enable Docker
    systemctl start docker
    systemctl enable docker
else
    echo -e "${YELLOW}Docker already installed${NC}"
fi

# Install Docker Compose
if ! command -v docker-compose &> /dev/null; then
    echo -e "${GREEN}Installing Docker Compose...${NC}"
    curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
else
    echo -e "${YELLOW}Docker Compose already installed${NC}"
fi

# Install useful tools
echo -e "${GREEN}Installing additional tools...${NC}"
apt-get install -y \
    curl \
    wget \
    git \
    vim \
    htop \
    net-tools

# Configure firewall (UFW)
echo -e "${GREEN}Configuring firewall...${NC}"
apt-get install -y ufw

# Allow SSH
ufw allow 22/tcp

# Allow HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp

# Allow proxy ports
ufw allow 1111/tcp
ufw allow 2222/tcp

# Enable firewall
echo "y" | ufw enable

# Show Docker version
echo -e "${BLUE}=== Installed Versions ===${NC}"
docker --version
docker-compose --version

echo -e "${GREEN}=== VPS Setup Complete ===${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Clone your repository"
echo -e "  2. Create .env file with your configuration"
echo -e "  3. Run: ./scripts/deploy.sh"
