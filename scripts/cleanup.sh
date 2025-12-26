#!/bin/bash
set -e

echo "🧹 Docker FaaS Cleanup Script"
echo "============================="
echo ""

read -p "This will remove all functions, containers, and data. Continue? (y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Cancelled."
    exit 0
fi

echo ""
echo "🛑 Stopping Docker FaaS..."
docker-compose down 2>/dev/null || docker compose down 2>/dev/null || true

echo ""
echo "🗑️  Removing function containers..."
docker ps -a --filter "label=com.docker-faas.function" -q | xargs -r docker rm -f

echo ""
echo "🗑️  Removing volumes..."
docker volume rm docker-faas-data 2>/dev/null || true

echo ""
echo "🗑️  Removing network..."
docker network rm docker-faas-net 2>/dev/null || true

echo ""
echo "🗑️  Cleaning up local database..."
rm -f docker-faas.db

echo ""
echo "✅ Cleanup complete!"
echo ""
echo "To start fresh, run: ./scripts/quick-start.sh"
echo ""
