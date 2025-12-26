#!/bin/bash
set -e

echo "🚀 Docker FaaS Quick Start Script"
echo "=================================="
echo ""

# Check prerequisites
echo "📋 Checking prerequisites..."

if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found. Please install Docker first."
    exit 1
fi
echo "✅ Docker found"

if ! command -v docker-compose &> /dev/null; then
    echo "⚠️  docker-compose not found. Will use docker compose instead."
    COMPOSE_CMD="docker compose"
else
    COMPOSE_CMD="docker-compose"
    echo "✅ docker-compose found"
fi

if ! command -v faas-cli &> /dev/null; then
    echo "⚠️  faas-cli not found. Installing..."
    curl -sL https://cli.openfaas.com | sudo sh
    echo "✅ faas-cli installed"
else
    echo "✅ faas-cli found"
fi

echo ""
echo "🔧 Starting Docker FaaS..."
$COMPOSE_CMD up -d

echo ""
echo "⏳ Waiting for gateway to be ready..."
for i in {1..30}; do
    if curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
        echo "✅ Gateway is ready!"
        break
    fi
    if [ $i -eq 30 ]; then
        echo "❌ Gateway failed to start. Check logs with: $COMPOSE_CMD logs"
        exit 1
    fi
    sleep 1
done

echo ""
echo "🔐 Logging in to gateway..."
echo "admin" | faas-cli login --gateway http://localhost:8080 --username admin --password-stdin

echo ""
echo "📦 Deploying example function..."
faas-cli deploy \
  --image ghcr.io/openfaas/alpine:latest \
  --name hello \
  --gateway http://localhost:8080 \
  --env fprocess="cat"

echo ""
echo "⏳ Waiting for function to be ready..."
sleep 5

echo ""
echo "🧪 Testing function..."
RESULT=$(echo "Hello, Docker FaaS!" | faas-cli invoke hello --gateway http://localhost:8080)
echo "Result: $RESULT"

echo ""
echo "✅ Success! Docker FaaS is ready to use!"
echo ""
echo "📚 Quick reference:"
echo "  - Gateway UI: http://localhost:8080"
echo "  - Metrics: http://localhost:9090/metrics"
echo "  - List functions: faas-cli list"
echo "  - Deploy function: faas-cli deploy -f stack.yml"
echo "  - Invoke function: echo 'data' | faas-cli invoke <name>"
echo "  - Remove function: faas-cli remove <name>"
echo "  - View logs: faas-cli logs <name>"
echo "  - Stop gateway: $COMPOSE_CMD down"
echo ""
echo "📖 Documentation: https://github.com/docker-faas/docker-faas"
echo ""
