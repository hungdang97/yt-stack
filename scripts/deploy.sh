#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Deploying YT Stack ===${NC}"

# Check if .env exists
if [ ! -f .env ]; then
    echo -e "${RED}Error: .env file not found!${NC}"
    exit 1
fi

# Load environment variables
export $(cat .env | grep -v '^#' | xargs)

# Check if domain is set
if [ -z "$DOMAIN" ]; then
    echo -e "${RED}Error: DOMAIN not set in .env${NC}"
    exit 1
fi

echo -e "${YELLOW}Deploying to: $DOMAIN${NC}"

# Pull latest images (if using registry)
# echo -e "${GREEN}Pulling latest images...${NC}"
# docker-compose pull

# Stop existing containers
echo -e "${GREEN}Stopping existing containers...${NC}"
docker-compose down

# Start services
echo -e "${GREEN}Starting services...${NC}"
docker-compose up -d

# Wait for services to be healthy
echo -e "${GREEN}Waiting for services to be healthy...${NC}"
sleep 10

# Check service status
echo -e "${BLUE}=== Service Status ===${NC}"
docker-compose ps

# Test health endpoints
echo -e "${BLUE}=== Health Checks ===${NC}"

if docker exec yt-downloader wget --no-verbose --tries=1 --spider http://localhost:8080/health 2>&1 | grep -q "200 OK"; then
    echo -e "${GREEN}✓ yt-downloader: HEALTHY${NC}"
else
    echo -e "${RED}✗ yt-downloader: UNHEALTHY${NC}"
fi

if docker exec yt-extractor wget --no-verbose --tries=1 --spider http://localhost:8300/health 2>&1 | grep -q "200 OK"; then
    echo -e "${GREEN}✓ yt-extractor: HEALTHY${NC}"
else
    echo -e "${RED}✗ yt-extractor: UNHEALTHY${NC}"
fi

echo -e "${BLUE}=== Deployment Complete ===${NC}"
echo -e "${GREEN}Services are running at:${NC}"
echo -e "  • Main service: https://$DOMAIN"
echo -e "  • WARP proxy: http://$DOMAIN:1111 (user: ${WARP_USER})"
echo -e "  • Direct proxy: http://$DOMAIN:2222 (user: ${DIRECT_USER})"
echo ""
echo -e "${YELLOW}View logs with: docker-compose logs -f${NC}"
