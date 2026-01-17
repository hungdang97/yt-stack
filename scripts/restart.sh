#!/bin/bash
set -e

SERVICE=${1:-}

if [ -z "$SERVICE" ]; then
    echo "Restarting all services..."
    docker-compose restart
else
    echo "Restarting $SERVICE..."
    docker-compose restart "$SERVICE"
fi

echo "✓ Restart complete"
docker-compose ps
