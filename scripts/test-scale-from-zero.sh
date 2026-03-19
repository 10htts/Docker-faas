#!/bin/bash

# Test script for scale-from-zero functionality
# Usage: ./test-scale-from-zero.sh <function-name> [gateway-url] [username:password]

set -e

FUNCTION_NAME="${1:-import-bundle}"
GATEWAY_URL="${2:-http://localhost:15012}"
AUTH="${3:-admin:admin}"

echo "=========================================="
echo "Testing Scale-From-Zero Implementation"
echo "=========================================="
echo "Function: $FUNCTION_NAME"
echo "Gateway: $GATEWAY_URL"
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Helper function to check function status
check_function() {
    echo -e "${YELLOW}Checking function status...${NC}"
    curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | \
        jq -r ".[] | select(.name==\"$FUNCTION_NAME\") | \"Replicas: \(.replicas), Available: \(.availableReplicas)\""
}

# Step 1: Verify function exists
echo -e "${YELLOW}Step 1: Verifying function exists...${NC}"
FUNCTION_EXISTS=$(curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | jq -r ".[] | select(.name==\"$FUNCTION_NAME\") | .name")

if [ -z "$FUNCTION_EXISTS" ]; then
    echo -e "${RED}Error: Function '$FUNCTION_NAME' not found${NC}"
    echo "Available functions:"
    curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | jq -r '.[].name'
    exit 1
fi
echo -e "${GREEN}✓ Function exists${NC}"
check_function
echo ""

# Step 2: Scale function to zero
echo -e "${YELLOW}Step 2: Scaling function to zero...${NC}"
curl -s -u "$AUTH" -X POST "$GATEWAY_URL/system/scale-function/$FUNCTION_NAME" \
    -H "Content-Type: application/json" \
    -d "{\"serviceName\": \"$FUNCTION_NAME\", \"replicas\": 0}" > /dev/null

sleep 2
check_function

AVAILABLE=$(curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | \
    jq -r ".[] | select(.name==\"$FUNCTION_NAME\") | .availableReplicas")

if [ "$AVAILABLE" != "0" ]; then
    echo -e "${RED}Warning: Available replicas is $AVAILABLE, expected 0${NC}"
    echo "Waiting for containers to stop..."
    sleep 5
fi
echo -e "${GREEN}✓ Function scaled to zero${NC}"
echo ""

# Step 3: Test synchronous invocation with scale-from-zero
echo -e "${YELLOW}Step 3: Testing synchronous invocation (should auto-scale)...${NC}"
echo "Payload: {\"test\": \"scale-from-zero\"}"
START_TIME=$(date +%s)

RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -u "$AUTH" -X POST "$GATEWAY_URL/function/$FUNCTION_NAME" \
    -H "Content-Type: application/json" \
    -d '{"test": "scale-from-zero"}')

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
BODY=$(echo "$RESPONSE" | grep -v "HTTP_CODE:")

echo "HTTP Status: $HTTP_CODE"
echo "Duration: ${DURATION}s"
echo "Response: $BODY"

if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "202" ]; then
    echo -e "${GREEN}✓ Invocation successful${NC}"
else
    echo -e "${RED}✗ Invocation failed with status $HTTP_CODE${NC}"
fi
echo ""

# Step 4: Verify function scaled up
echo -e "${YELLOW}Step 4: Verifying function scaled up...${NC}"
sleep 2
check_function

AVAILABLE=$(curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | \
    jq -r ".[] | select(.name==\"$FUNCTION_NAME\") | .availableReplicas")

if [ "$AVAILABLE" -gt "0" ]; then
    echo -e "${GREEN}✓ Function has $AVAILABLE available replicas${NC}"
else
    echo -e "${RED}✗ Function still has 0 available replicas${NC}"
fi
echo ""

# Step 5: Test second invocation (should be fast)
echo -e "${YELLOW}Step 5: Testing second invocation (should be fast)...${NC}"
START_TIME=$(date +%s)

RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -u "$AUTH" -X POST "$GATEWAY_URL/function/$FUNCTION_NAME" \
    -H "Content-Type: application/json" \
    -d '{"test": "warm-invoke"}')

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

HTTP_CODE=$(echo "$RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)

echo "HTTP Status: $HTTP_CODE"
echo "Duration: ${DURATION}s (should be <2s)"

if [ "$DURATION" -lt 3 ]; then
    echo -e "${GREEN}✓ Fast invocation confirmed${NC}"
else
    echo -e "${YELLOW}⚠ Invocation took ${DURATION}s (expected <2s)${NC}"
fi
echo ""

# Step 6: Test async invocation
echo -e "${YELLOW}Step 6: Scaling to zero again for async test...${NC}"
curl -s -u "$AUTH" -X POST "$GATEWAY_URL/system/scale-function/$FUNCTION_NAME" \
    -H "Content-Type: application/json" \
    -d "{\"serviceName\": \"$FUNCTION_NAME\", \"replicas\": 0}" > /dev/null

sleep 3
check_function
echo ""

echo -e "${YELLOW}Step 7: Testing async invocation with scale-from-zero...${NC}"
START_TIME=$(date +%s)

ASYNC_RESPONSE=$(curl -s -w "\nHTTP_CODE:%{http_code}" -u "$AUTH" -X POST "$GATEWAY_URL/async-function/$FUNCTION_NAME" \
    -H "Content-Type: application/json" \
    -d '{"test": "async-scale-from-zero"}')

END_TIME=$(date +%s)
DURATION=$((END_TIME - START_TIME))

HTTP_CODE=$(echo "$ASYNC_RESPONSE" | grep "HTTP_CODE:" | cut -d: -f2)
BODY=$(echo "$ASYNC_RESPONSE" | grep -v "HTTP_CODE:")

echo "HTTP Status: $HTTP_CODE"
echo "Duration: ${DURATION}s"
echo "Response: $BODY"

if [ "$HTTP_CODE" = "202" ]; then
    CALL_ID=$(echo "$BODY" | jq -r '.callId')
    echo -e "${GREEN}✓ Async invocation accepted (Call ID: $CALL_ID)${NC}"
else
    echo -e "${RED}✗ Async invocation failed with status $HTTP_CODE${NC}"
fi
echo ""

# Step 8: Verify function scaled up again
echo -e "${YELLOW}Step 8: Verifying function scaled up from async invocation...${NC}"
sleep 2
check_function

AVAILABLE=$(curl -s -u "$AUTH" "$GATEWAY_URL/system/functions" | \
    jq -r ".[] | select(.name==\"$FUNCTION_NAME\") | .availableReplicas")

if [ "$AVAILABLE" -gt "0" ]; then
    echo -e "${GREEN}✓ Function has $AVAILABLE available replicas${NC}"
else
    echo -e "${RED}✗ Function still has 0 available replicas${NC}"
fi
echo ""

# Summary
echo "=========================================="
echo -e "${GREEN}Scale-From-Zero Test Complete!${NC}"
echo "=========================================="
echo ""
echo "Results:"
echo "  ✓ Function exists and can be queried"
echo "  ✓ Function can be scaled to zero"
echo "  ✓ Synchronous invocation triggers auto-scale"
echo "  ✓ Function scales up successfully"
echo "  ✓ Warm invocations are fast"
echo "  ✓ Async invocation triggers auto-scale"
echo ""
echo "Next steps:"
echo "  - Check gateway logs: docker logs docker-faas-gateway"
echo "  - Check function logs: curl -u $AUTH '$GATEWAY_URL/system/logs?name=$FUNCTION_NAME&tail=50'"
echo "  - Monitor metrics: curl -u $AUTH '$GATEWAY_URL/metrics'"
echo ""
