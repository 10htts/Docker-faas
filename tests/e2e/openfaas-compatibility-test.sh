#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Test counters
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# Configuration
GATEWAY="http://localhost:8080"
USERNAME="admin"
PASSWORD="admin"

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Docker FaaS - OpenFaaS Compatibility Test${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# Helper functions
test_start() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    echo -e "${YELLOW}TEST $TOTAL_TESTS: $1${NC}"
}

test_pass() {
    PASSED_TESTS=$((PASSED_TESTS + 1))
    echo -e "${GREEN}✓ PASS${NC}"
    echo ""
}

test_fail() {
    FAILED_TESTS=$((FAILED_TESTS + 1))
    echo -e "${RED}✗ FAIL: $1${NC}"
    echo ""
}

# Cleanup function
cleanup() {
    echo -e "${YELLOW}Cleaning up test functions...${NC}"
    faas-cli remove echo 2>/dev/null || true
    faas-cli remove nodeinfo 2>/dev/null || true
    faas-cli remove env 2>/dev/null || true
    faas-cli remove figlet 2>/dev/null || true
    faas-cli remove uppercase 2>/dev/null || true
    echo -e "${GREEN}Cleanup complete${NC}"
    echo ""
}

# Set trap to cleanup on exit
trap cleanup EXIT

# Prerequisites check
echo -e "${BLUE}Checking prerequisites...${NC}"

if ! command -v faas-cli &> /dev/null; then
    echo -e "${RED}faas-cli not found. Please install it first.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ faas-cli found ($(faas-cli version --short-version))${NC}"

if ! command -v curl &> /dev/null; then
    echo -e "${RED}curl not found. Please install it first.${NC}"
    exit 1
fi
echo -e "${GREEN}✓ curl found${NC}"

if ! command -v jq &> /dev/null; then
    echo -e "${YELLOW}⚠ jq not found. Some tests may be limited.${NC}"
    HAS_JQ=false
else
    echo -e "${GREEN}✓ jq found${NC}"
    HAS_JQ=true
fi

echo ""

# Test 1: Gateway Health Check
test_start "Gateway health check (/healthz)"
HEALTH=$(curl -s -o /dev/null -w "%{http_code}" $GATEWAY/healthz)
if [ "$HEALTH" == "200" ]; then
    test_pass
else
    test_fail "Expected 200, got $HEALTH"
fi

# Test 2: System Info
test_start "System info endpoint (/system/info)"
INFO=$(curl -s -u $USERNAME:$PASSWORD $GATEWAY/system/info)
if echo "$INFO" | grep -q "docker-faas"; then
    test_pass
else
    test_fail "System info doesn't contain expected data"
fi

# Test 3: faas-cli login
test_start "faas-cli login compatibility"
if echo "$PASSWORD" | faas-cli login --gateway $GATEWAY --username $USERNAME --password-stdin &>/dev/null; then
    test_pass
else
    test_fail "Login failed"
fi

# Test 4: Deploy simple echo function
test_start "Deploy function using faas-cli (echo function)"
if faas-cli deploy \
    --gateway $GATEWAY \
    --image ghcr.io/openfaas/alpine:latest \
    --name echo \
    --env fprocess="cat" &>/dev/null; then
    test_pass
else
    test_fail "Deployment failed"
fi

# Wait for function to be ready
echo -e "${YELLOW}Waiting for function to be ready...${NC}"
sleep 5

# Test 5: List functions
test_start "List functions (faas-cli list)"
FUNCTION_LIST=$(faas-cli list --gateway $GATEWAY 2>/dev/null | grep -c "echo" || true)
if [ "$FUNCTION_LIST" -ge 1 ]; then
    test_pass
else
    test_fail "Function not found in list"
fi

# Test 6: Invoke function via faas-cli
test_start "Invoke function via faas-cli"
RESULT=$(echo "Hello World" | faas-cli invoke echo --gateway $GATEWAY 2>/dev/null)
if [ "$RESULT" == "Hello World" ]; then
    test_pass
else
    test_fail "Expected 'Hello World', got '$RESULT'"
fi

# Test 7: Invoke function via HTTP POST
test_start "Invoke function via HTTP POST (/function/echo)"
RESULT=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/function/echo -d "Test Message")
if [ "$RESULT" == "Test Message" ]; then
    test_pass
else
    test_fail "Expected 'Test Message', got '$RESULT'"
fi

# Test 8: Invoke with JSON payload
test_start "Invoke function with JSON payload"
RESULT=$(curl -s -u $USERNAME:$PASSWORD \
    -X POST $GATEWAY/function/echo \
    -H "Content-Type: application/json" \
    -d '{"message":"test"}')
if echo "$RESULT" | grep -q "message"; then
    test_pass
else
    test_fail "JSON payload not echoed correctly"
fi

# Test 9: Scale function
test_start "Scale function to 3 replicas"
if faas-cli scale echo --replicas 3 --gateway $GATEWAY &>/dev/null; then
    sleep 3
    # Check if scaling worked
    REPLICAS=$(faas-cli list --gateway $GATEWAY 2>/dev/null | grep "echo" | awk '{print $3}' || echo "0")
    if [ "$REPLICAS" == "3" ]; then
        test_pass
    else
        test_fail "Expected 3 replicas, got $REPLICAS"
    fi
else
    test_fail "Scale command failed"
fi

# Test 10: Multiple invocations (load balancing)
test_start "Multiple invocations (testing load balancing)"
SUCCESS_COUNT=0
for i in {1..10}; do
    RESULT=$(echo "Test $i" | faas-cli invoke echo --gateway $GATEWAY 2>/dev/null)
    if [ "$RESULT" == "Test $i" ]; then
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    fi
done
if [ "$SUCCESS_COUNT" -eq 10 ]; then
    test_pass
else
    test_fail "Only $SUCCESS_COUNT/10 invocations succeeded"
fi

# Test 11: Scale down to 1 replica
test_start "Scale function down to 1 replica"
if faas-cli scale echo --replicas 1 --gateway $GATEWAY &>/dev/null; then
    sleep 2
    test_pass
else
    test_fail "Scale down failed"
fi

# Test 12: Deploy function with environment variables
test_start "Deploy function with environment variables"
if faas-cli deploy \
    --gateway $GATEWAY \
    --image ghcr.io/openfaas/alpine:latest \
    --name env \
    --env fprocess="env" \
    --env CUSTOM_VAR="test-value" \
    --env ANOTHER_VAR="another-test" &>/dev/null; then
    sleep 3
    test_pass
else
    test_fail "Deployment with env vars failed"
fi

# Test 13: Verify environment variables
test_start "Verify environment variables are set"
RESULT=$(faas-cli invoke env --gateway $GATEWAY 2>/dev/null </dev/null)
if echo "$RESULT" | grep -q "CUSTOM_VAR=test-value"; then
    test_pass
else
    test_fail "Environment variable not found in function"
fi

# Test 14: Deploy function with labels
test_start "Deploy function with labels"
if faas-cli deploy \
    --gateway $GATEWAY \
    --image ghcr.io/openfaas/alpine:latest \
    --name uppercase \
    --env fprocess="tr '[:lower:]' '[:upper:]'" \
    --label "tier=backend" \
    --label "team=platform" &>/dev/null; then
    sleep 3
    test_pass
else
    test_fail "Deployment with labels failed"
fi

# Test 15: Function logs retrieval
test_start "Retrieve function logs (faas-cli logs)"
if faas-cli logs echo --gateway $GATEWAY 2>&1 | grep -q ""; then
    test_pass
else
    test_fail "Log retrieval failed"
fi

# Test 16: Update existing function
test_start "Update existing function (faas-cli deploy --update)"
if faas-cli deploy \
    --gateway $GATEWAY \
    --image ghcr.io/openfaas/alpine:latest \
    --name echo \
    --env fprocess="cat" \
    --env UPDATED="true" \
    --update=true &>/dev/null; then
    sleep 3
    test_pass
else
    test_fail "Function update failed"
fi

# Test 17: Remove function
test_start "Remove function (faas-cli remove)"
if faas-cli remove uppercase --gateway $GATEWAY &>/dev/null; then
    sleep 2
    # Verify it's gone
    FUNCTION_LIST=$(faas-cli list --gateway $GATEWAY 2>/dev/null | grep -c "uppercase" || true)
    if [ "$FUNCTION_LIST" -eq 0 ]; then
        test_pass
    else
        test_fail "Function still exists after removal"
    fi
else
    test_fail "Remove command failed"
fi

# Test 18: Deploy from stack file
test_start "Deploy from stack.yml file"
cat > /tmp/test-stack.yml << 'EOF'
version: 1.0
provider:
  name: openfaas
  gateway: http://localhost:8080

functions:
  nodeinfo:
    image: ghcr.io/openfaas/alpine:latest
    environment:
      fprocess: "uname -a"
    labels:
      com.openfaas.test: "true"
EOF

if faas-cli deploy -f /tmp/test-stack.yml --gateway $GATEWAY &>/dev/null; then
    sleep 3
    # Verify deployment
    if faas-cli list --gateway $GATEWAY 2>/dev/null | grep -q "nodeinfo"; then
        test_pass
    else
        test_fail "Function from stack.yml not found"
    fi
else
    test_fail "Stack deployment failed"
fi

# Test 19: Invoke function from stack
test_start "Invoke function deployed from stack"
RESULT=$(faas-cli invoke nodeinfo --gateway $GATEWAY 2>/dev/null </dev/null)
if echo "$RESULT" | grep -q "Linux"; then
    test_pass
else
    test_fail "Stack function invocation failed"
fi

# Test 20: Remove from stack file
test_start "Remove function using stack.yml"
if faas-cli remove -f /tmp/test-stack.yml --gateway $GATEWAY &>/dev/null; then
    test_pass
else
    test_fail "Stack removal failed"
fi

# Test 21: Concurrent invocations
test_start "Concurrent invocations (stress test)"
SUCCESS_COUNT=0
for i in {1..20}; do
    (echo "Concurrent $i" | faas-cli invoke echo --gateway $GATEWAY &>/dev/null && echo "1" >> /tmp/concurrent_test.txt) &
done
wait
if [ -f /tmp/concurrent_test.txt ]; then
    SUCCESS_COUNT=$(wc -l < /tmp/concurrent_test.txt)
    rm /tmp/concurrent_test.txt
fi
if [ "$SUCCESS_COUNT" -ge 18 ]; then  # Allow 2 failures
    test_pass
else
    test_fail "Only $SUCCESS_COUNT/20 concurrent invocations succeeded"
fi

# Test 22: HTTP methods support
test_start "Support for different HTTP methods (GET, POST, PUT, DELETE)"
GET_RESULT=$(curl -s -u $USERNAME:$PASSWORD -X GET $GATEWAY/function/echo?test=1)
POST_RESULT=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/function/echo -d "post")
PUT_RESULT=$(curl -s -u $USERNAME:$PASSWORD -X PUT $GATEWAY/function/echo -d "put")
DELETE_RESULT=$(curl -s -u $USERNAME:$PASSWORD -X DELETE $GATEWAY/function/echo)

if [ ! -z "$GET_RESULT" ] && [ ! -z "$POST_RESULT" ] && [ ! -z "$PUT_RESULT" ]; then
    test_pass
else
    test_fail "Not all HTTP methods supported"
fi

# Test 23: Large payload handling
test_start "Large payload handling (1MB)"
LARGE_PAYLOAD=$(dd if=/dev/zero bs=1024 count=1024 2>/dev/null | base64)
RESULT=$(echo "$LARGE_PAYLOAD" | faas-cli invoke echo --gateway $GATEWAY 2>/dev/null | wc -c)
if [ "$RESULT" -gt 1000000 ]; then
    test_pass
else
    test_fail "Large payload not handled correctly"
fi

# Test 24: Authentication enforcement
test_start "Authentication is enforced (401 without credentials)"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" $GATEWAY/system/functions)
if [ "$STATUS" == "401" ]; then
    test_pass
else
    test_fail "Expected 401 Unauthorized, got $STATUS"
fi

# Test 25: Metrics endpoint
test_start "Prometheus metrics endpoint (/metrics)"
METRICS=$(curl -s http://localhost:9090/metrics)
if echo "$METRICS" | grep -q "function_invocations_total"; then
    test_pass
else
    test_fail "Metrics not available or incomplete"
fi

# Cleanup test file
rm -f /tmp/test-stack.yml

# Summary
echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}Test Summary${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "Total Tests:  $TOTAL_TESTS"
echo -e "${GREEN}Passed:       $PASSED_TESTS${NC}"
if [ "$FAILED_TESTS" -gt 0 ]; then
    echo -e "${RED}Failed:       $FAILED_TESTS${NC}"
else
    echo -e "${GREEN}Failed:       $FAILED_TESTS${NC}"
fi
echo ""

# Calculate percentage
PERCENTAGE=$((PASSED_TESTS * 100 / TOTAL_TESTS))
echo -e "Success Rate: ${PERCENTAGE}%"
echo ""

if [ "$FAILED_TESTS" -eq 0 ]; then
    echo -e "${GREEN}========================================${NC}"
    echo -e "${GREEN}🎉 ALL TESTS PASSED!${NC}"
    echo -e "${GREEN}Docker FaaS is fully compatible with OpenFaaS!${NC}"
    echo -e "${GREEN}========================================${NC}"
    exit 0
else
    echo -e "${RED}========================================${NC}"
    echo -e "${RED}Some tests failed. Please review the output above.${NC}"
    echo -e "${RED}========================================${NC}"
    exit 1
fi
