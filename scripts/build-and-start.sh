#!/bin/bash

# Quick Build and Start Script
# Build all images and start the stack

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "========================================="
echo "YouTube Downloader - Build & Start"
echo "========================================="
echo ""

# Check if .env exists
if [ ! -f ".env" ]; then
    echo -e "${YELLOW}⚠${NC} .env file not found!"
    echo "Copying from .env.example..."
    cp .env.example .env
    echo ""
    echo -e "${YELLOW}⚠${NC} Please edit .env file and set your DOMAIN and EMAIL:"
    echo "  nano .env"
    echo ""
    echo "Then run this script again."
    exit 1
fi

# Check DOMAIN is set
DOMAIN=$(grep "^DOMAIN=" .env | cut -d '=' -f2)
if [ "$DOMAIN" == "your-domain.com" ] || [ -z "$DOMAIN" ]; then
    echo -e "${YELLOW}⚠${NC} Please update DOMAIN in .env file first:"
    echo "  nano .env"
    exit 1
fi

echo -e "${GREEN}✓${NC} Using domain: $DOMAIN"
echo ""

# Build
echo -e "${YELLOW}ℹ${NC} Building Docker images (this may take 5-10 minutes)..."
docker compose build --no-cache

echo ""
echo -e "${GREEN}✓${NC} Build completed!"
echo ""

# Start
echo -e "${YELLOW}ℹ${NC} Starting services..."
docker compose up -d

echo ""
echo -e "${GREEN}✓${NC} Services started!"
echo ""

# Wait a bit for containers to initialize
echo "Waiting for containers to initialize..."
sleep 5

# Show status
echo ""
echo "Container Status:"
docker compose ps

echo ""
echo "========================================="
echo -e "${GREEN}Deployment Complete!${NC}"
echo "========================================="
echo ""
echo "Monitor logs:"
echo "  docker compose logs -f"
echo ""
echo "Test your deployment:"
echo "  curl https://$DOMAIN/health"
echo ""
