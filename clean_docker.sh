#!/bin/bash
echo "=== Docker Deep Clean ==="
echo "WARNING: This will remove:"
echo "- All stopped containers"
echo "- All networks not used by at least one container"
echo "- All images without at least one container associated to them"
echo "- All build cache"
echo ""
read -p "Are you sure? (y/N) " confirm
if [[ $confirm == [yY] || $confirm == [yY][eE][sS] ]]; then
    docker system prune -af --volumes
    echo "✓ Cleanup complete."
else
    echo "Cancelled."
fi
