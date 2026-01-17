#!/bin/bash
set -e

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}=== Building Docker Images ===${NC}"

# Load environment variables
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

# Build all services
echo -e "${GREEN}Building yt-downloader...${NC}"
docker-compose build yt-downloader

echo -e "${GREEN}Building yt-extractor...${NC}"
docker-compose build yt-extractor

echo -e "${GREEN}Building nginx...${NC}"
docker-compose build nginx

echo -e "${GREEN}Build completed successfully!${NC}"
echo -e "${BLUE}=== Images ready to deploy ===${NC}"
docker images | grep -E 'yt-downloader|yt-extractor|nginx-ssl'
