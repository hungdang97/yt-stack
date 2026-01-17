#!/bin/bash

# Show logs for specific service or all services
SERVICE=${1:-}

if [ -z "$SERVICE" ]; then
    echo "Showing logs for all services..."
    docker-compose logs -f --tail=100
else
    echo "Showing logs for $SERVICE..."
    docker-compose logs -f --tail=100 "$SERVICE"
fi
