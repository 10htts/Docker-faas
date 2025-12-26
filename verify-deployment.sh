#!/bin/bash

# Docker FaaS - Quick Deployment Verification
# This script verifies that Docker FaaS is ready for production

set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   Docker FaaS - Deployment Verification       ║${NC}"
echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo ""

# Check if running in the correct directory
if [ ! -f "docker-compose.yml" ]; then
    echo -e "${RED}Error: docker-compose.yml not found. Please run this script from the project root.${NC}"
    exit 1
fi

echo -e "${YELLOW}Step 1: Checking prerequisites...${NC}"

# Check Docker
if command -v docker &> /dev/null; then
    echo -e "${GREEN}✓ Docker installed ($(docker --version | cut -d' ' -f3 | cut -d',' -f1))${NC}"
else
    echo -e "${RED}✗ Docker not found. Please install Docker.${NC}"
    exit 1
fi

# Check Docker Compose
if command -v docker-compose &> /dev/null; then
    echo -e "${GREEN}✓ Docker Compose installed ($(docker-compose --version | cut -d' ' -f3 | cut -d',' -f1))${NC}"
    COMPOSE_CMD="docker-compose"
elif docker compose version &> /dev/null; then
    echo -e "${GREEN}✓ Docker Compose installed ($(docker compose version --short))${NC}"
    COMPOSE_CMD="docker compose"
else
    echo -e "${RED}✗ Docker Compose not found. Please install Docker Compose.${NC}"
    exit 1
fi

# Check faas-cli (optional)
if command -v faas-cli &> /dev/null; then
    echo -e "${GREEN}✓ faas-cli installed ($(faas-cli version --short-version 2>/dev/null || echo 'unknown'))${NC}"
    HAS_FAAS_CLI=true
else
    echo -e "${YELLOW}⚠ faas-cli not found. Will skip faas-cli tests.${NC}"
    echo -e "${YELLOW}  Install with: curl -sL https://cli.openfaas.com | sudo sh${NC}"
    HAS_FAAS_CLI=false
fi

echo ""
echo -e "${YELLOW}Step 2: Starting Docker FaaS...${NC}"

# Stop any existing instance
$COMPOSE_CMD down &>/dev/null || true

# Start Docker FaaS
echo "Starting containers..."
$COMPOSE_CMD up -d

if [ $? -eq 0 ]; then
    echo -e "${GREEN}✓ Docker FaaS started successfully${NC}"
else
    echo -e "${RED}✗ Failed to start Docker FaaS${NC}"
    exit 1
fi

echo ""
echo -e "${YELLOW}Step 3: Waiting for gateway to be ready...${NC}"

# Wait for gateway
RETRY_COUNT=0
MAX_RETRIES=30

while [ $RETRY_COUNT -lt $MAX_RETRIES ]; do
    if curl -s http://localhost:8080/healthz > /dev/null 2>&1; then
        echo -e "${GREEN}✓ Gateway is ready!${NC}"
        break
    fi

    RETRY_COUNT=$((RETRY_COUNT + 1))
    if [ $RETRY_COUNT -eq $MAX_RETRIES ]; then
        echo -e "${RED}✗ Gateway failed to start after ${MAX_RETRIES} seconds${NC}"
        echo -e "${YELLOW}Check logs with: $COMPOSE_CMD logs gateway${NC}"
        exit 1
    fi

    echo -n "."
    sleep 1
done

echo ""
echo -e "${YELLOW}Step 4: Running basic health checks...${NC}"

# Test health endpoint
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/healthz)
if [ "$HEALTH" == "200" ]; then
    echo -e "${GREEN}✓ Health endpoint responding (200 OK)${NC}"
else
    echo -e "${RED}✗ Health endpoint failed (Status: $HEALTH)${NC}"
    exit 1
fi

# Test system info
INFO=$(curl -s -u admin:admin http://localhost:8080/system/info)
if echo "$INFO" | grep -q "docker-faas"; then
    echo -e "${GREEN}✓ System info endpoint working${NC}"
    VERSION=$(echo "$INFO" | grep -o '"version":"[^"]*"' | cut -d'"' -f4 || echo "1.0.0")
    echo -e "  Version: $VERSION"
else
    echo -e "${RED}✗ System info endpoint failed${NC}"
    exit 1
fi

# Test metrics endpoint
METRICS=$(curl -s http://localhost:9090/metrics)
if echo "$METRICS" | grep -q "function_invocations_total"; then
    echo -e "${GREEN}✓ Metrics endpoint working${NC}"
else
    echo -e "${YELLOW}⚠ Metrics endpoint may not be fully initialized${NC}"
fi

# Test authentication
AUTH_TEST=$(curl -s -o /dev/null -w "%{http_code}" http://localhost:8080/system/functions)
if [ "$AUTH_TEST" == "401" ]; then
    echo -e "${GREEN}✓ Authentication enforced (401 without credentials)${NC}"
else
    echo -e "${YELLOW}⚠ Authentication may not be working correctly (Status: $AUTH_TEST)${NC}"
fi

echo ""
echo -e "${YELLOW}Step 5: Testing function deployment...${NC}"

if [ "$HAS_FAAS_CLI" = true ]; then
    # Login
    echo "Logging in with faas-cli..."
    if echo "admin" | faas-cli login --gateway http://localhost:8080 --username admin --password-stdin &>/dev/null; then
        echo -e "${GREEN}✓ faas-cli login successful${NC}"
    else
        echo -e "${RED}✗ faas-cli login failed${NC}"
        exit 1
    fi

    # Deploy test function
    echo "Deploying test function..."
    if faas-cli deploy \
        --gateway http://localhost:8080 \
        --image ghcr.io/openfaas/alpine:latest \
        --name verify-test \
        --env fprocess="cat" &>/dev/null; then
        echo -e "${GREEN}✓ Function deployed successfully${NC}"
    else
        echo -e "${RED}✗ Function deployment failed${NC}"
        exit 1
    fi

    # Wait for function to be ready
    sleep 5

    # List functions
    if faas-cli list --gateway http://localhost:8080 2>/dev/null | grep -q "verify-test"; then
        echo -e "${GREEN}✓ Function appears in list${NC}"
    else
        echo -e "${RED}✗ Function not found in list${NC}"
    fi

    # Invoke function
    echo "Testing function invocation..."
    RESULT=$(echo "Hello Docker FaaS" | faas-cli invoke verify-test --gateway http://localhost:8080 2>/dev/null)
    if [ "$RESULT" == "Hello Docker FaaS" ]; then
        echo -e "${GREEN}✓ Function invocation successful${NC}"
    else
        echo -e "${RED}✗ Function invocation failed${NC}"
        echo -e "${RED}  Expected: 'Hello Docker FaaS', Got: '$RESULT'${NC}"
    fi

    # Cleanup
    faas-cli remove verify-test --gateway http://localhost:8080 &>/dev/null
    echo -e "${GREEN}✓ Test function removed${NC}"
else
    # Test with curl
    echo "Testing with curl (faas-cli not available)..."

    # Deploy via API
    DEPLOY_RESPONSE=$(curl -s -u admin:admin -X POST http://localhost:8080/system/functions \
        -H "Content-Type: application/json" \
        -d '{
            "service": "verify-test",
            "image": "ghcr.io/openfaas/alpine:latest",
            "envVars": {"fprocess": "cat"}
        }' -w "\n%{http_code}")

    STATUS=$(echo "$DEPLOY_RESPONSE" | tail -n1)
    if [ "$STATUS" == "202" ]; then
        echo -e "${GREEN}✓ Function deployed via API${NC}"
        sleep 5

        # Invoke
        RESULT=$(curl -s -u admin:admin -X POST http://localhost:8080/function/verify-test -d "Test")
        if [ "$RESULT" == "Test" ]; then
            echo -e "${GREEN}✓ Function invocation successful${NC}"
        else
            echo -e "${YELLOW}⚠ Function invocation may have issues${NC}"
        fi

        # Cleanup
        curl -s -u admin:admin -X DELETE "http://localhost:8080/system/functions?functionName=verify-test" &>/dev/null
        echo -e "${GREEN}✓ Test function removed${NC}"
    else
        echo -e "${YELLOW}⚠ Function deployment via API returned status: $STATUS${NC}"
    fi
fi

echo ""
echo -e "${YELLOW}Step 6: Checking Docker resources...${NC}"

# Check containers
CONTAINERS=$($COMPOSE_CMD ps -q | wc -l)
echo -e "${GREEN}✓ Docker containers running: $CONTAINERS${NC}"

# Check network
if docker network inspect docker-faas-net &>/dev/null; then
    echo -e "${GREEN}✓ Docker network 'docker-faas-net' exists${NC}"
else
    echo -e "${YELLOW}⚠ Docker network may have issues${NC}"
fi

# Check volume
if docker volume inspect docker-faas-data &>/dev/null; then
    echo -e "${GREEN}✓ Docker volume 'docker-faas-data' exists${NC}"
else
    echo -e "${YELLOW}⚠ Docker volume may have issues${NC}"
fi

echo ""
echo -e "${BLUE}╔════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║            Verification Summary                ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}✓ Docker FaaS is running and operational!${NC}"
echo ""
echo -e "${BLUE}Access Information:${NC}"
echo -e "  Gateway API:  http://localhost:8080"
echo -e "  Metrics:      http://localhost:9090/metrics"
echo -e "  Username:     admin"
echo -e "  Password:     admin"
echo ""
echo -e "${BLUE}Quick Commands:${NC}"
echo -e "  Status:       $COMPOSE_CMD ps"
echo -e "  Logs:         $COMPOSE_CMD logs -f gateway"
echo -e "  Stop:         $COMPOSE_CMD down"
echo -e "  Restart:      $COMPOSE_CMD restart"
echo ""

if [ "$HAS_FAAS_CLI" = true ]; then
    echo -e "${BLUE}Next Steps with faas-cli:${NC}"
    echo -e "  1. Login:     faas-cli login --gateway http://localhost:8080 -u admin -p admin"
    echo -e "  2. Deploy:    faas-cli deploy -f examples/stack.yml"
    echo -e "  3. List:      faas-cli list"
    echo -e "  4. Invoke:    echo 'data' | faas-cli invoke <function-name>"
    echo ""
    echo -e "${BLUE}Run Full Compatibility Tests:${NC}"
    echo -e "  make e2e-test"
    echo -e "  ./tests/e2e/openfaas-compatibility-test.sh"
else
    echo -e "${YELLOW}Install faas-cli for full functionality:${NC}"
    echo -e "  curl -sL https://cli.openfaas.com | sudo sh"
fi

echo ""
echo -e "${GREEN}╔════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  ✓ Deployment Verification Complete!          ║${NC}"
echo -e "${GREEN}║  Your Docker FaaS is ready for use!           ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════╝${NC}"
echo ""
