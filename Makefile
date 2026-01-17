.PHONY: help build up down restart logs ps clean setup-vps

# Default target
help:
	@echo "Available commands:"
	@echo "  make build      - Build all Docker images"
	@echo "  make up         - Start all services"
	@echo "  make down       - Stop all services"
	@echo "  make restart    - Restart all services"
	@echo "  make logs       - View logs (all services)"
	@echo "  make ps         - Show service status"
	@echo "  make clean      - Remove all containers and volumes"
	@echo "  make setup-vps  - Setup VPS (first time only)"

# Build all images
build:
	@bash scripts/build.sh

# Start all services
up:
	@docker-compose up -d
	@echo "✓ Services started"
	@docker-compose ps

# Stop all services
down:
	@docker-compose down
	@echo "✓ Services stopped"

# Restart all services
restart:
	@docker-compose restart
	@echo "✓ Services restarted"
	@docker-compose ps

# View logs
logs:
	@docker-compose logs -f --tail=100

# Show service status
ps:
	@docker-compose ps

# Clean up everything
clean:
	@docker-compose down -v
	@echo "✓ Containers and volumes removed"

# Setup VPS (run once)
setup-vps:
	@sudo bash scripts/setup-vps.sh
