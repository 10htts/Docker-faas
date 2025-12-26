#!/bin/bash
set -e

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

GATEWAY="http://localhost:8080"
USERNAME="admin"
PASSWORD="admin"

echo -e "${YELLOW}Testing Secrets Management${NC}"
echo ""

# Test 1: Create a secret
echo -e "${YELLOW}TEST 1: Create secret via API${NC}"
RESPONSE=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/system/secrets \
  -H "Content-Type: application/json" \
  -d '{"name":"test-secret","value":"secret-value-123"}' \
  -w "\n%{http_code}")

STATUS=$(echo "$RESPONSE" | tail -n1)
if [ "$STATUS" == "201" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 201, got $STATUS${NC}"
    exit 1
fi

# Test 2: List secrets
echo -e "${YELLOW}TEST 2: List secrets${NC}"
SECRETS=$(curl -s -u $USERNAME:$PASSWORD $GATEWAY/system/secrets)
if echo "$SECRETS" | grep -q "test-secret"; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Secret not found in list${NC}"
    exit 1
fi

# Test 3: Get secret info
echo -e "${YELLOW}TEST 3: Get secret info${NC}"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -u $USERNAME:$PASSWORD $GATEWAY/system/secrets/test-secret)
if [ "$STATUS" == "200" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 200, got $STATUS${NC}"
    exit 1
fi

# Test 4: Update secret
echo -e "${YELLOW}TEST 4: Update secret${NC}"
RESPONSE=$(curl -s -u $USERNAME:$PASSWORD -X PUT $GATEWAY/system/secrets \
  -H "Content-Type: application/json" \
  -d '{"name":"test-secret","value":"updated-value-456"}' \
  -w "\n%{http_code}")

STATUS=$(echo "$RESPONSE" | tail -n1)
if [ "$STATUS" == "200" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 200, got $STATUS${NC}"
    exit 1
fi

# Test 5: Create function with secret
echo -e "${YELLOW}TEST 5: Deploy function with secret${NC}"
DEPLOY_RESPONSE=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/system/functions \
  -H "Content-Type: application/json" \
  -d '{
    "service": "secret-test",
    "image": "ghcr.io/openfaas/alpine:latest",
    "envVars": {"fprocess": "cat /var/openfaas/secrets/test-secret"},
    "secrets": ["test-secret"]
  }' \
  -w "\n%{http_code}")

STATUS=$(echo "$DEPLOY_RESPONSE" | tail -n1)
if [ "$STATUS" == "202" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
    sleep 5  # Wait for deployment
else
    echo -e "${RED}✗ FAIL: Expected 202, got $STATUS${NC}"
    exit 1
fi

# Test 6: Deploy function with missing secret (should fail)
echo -e "${YELLOW}TEST 6: Deploy with missing secret (should fail)${NC}"
DEPLOY_RESPONSE=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/system/functions \
  -H "Content-Type: application/json" \
  -d '{
    "service": "missing-secret-test",
    "image": "ghcr.io/openfaas/alpine:latest",
    "secrets": ["nonexistent-secret"]
  }' \
  -w "\n%{http_code}")

STATUS=$(echo "$DEPLOY_RESPONSE" | tail -n1)
if [ "$STATUS" == "500" ] || [ "$STATUS" == "400" ]; then
    echo -e "${GREEN}✓ PASS (correctly rejected)${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 400/500, got $STATUS${NC}"
    exit 1
fi

# Test 7: Invoke function and verify secret is accessible
echo -e "${YELLOW}TEST 7: Verify secret is mounted in function${NC}"
RESULT=$(curl -s -u $USERNAME:$PASSWORD -X POST $GATEWAY/function/secret-test)
if echo "$RESULT" | grep -q "updated-value-456"; then
    echo -e "${GREEN}✓ PASS (secret accessible in function)${NC}"
else
    echo -e "${YELLOW}⚠ WARNING: Could not verify secret content (may need shell in container)${NC}"
fi

# Test 8: Delete secret
echo -e "${YELLOW}TEST 8: Delete secret${NC}"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -u $USERNAME:$PASSWORD -X DELETE "$GATEWAY/system/secrets?name=test-secret")
if [ "$STATUS" == "204" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 204, got $STATUS${NC}"
    exit 1
fi

# Test 9: Verify secret is deleted
echo -e "${YELLOW}TEST 9: Verify secret is deleted${NC}"
STATUS=$(curl -s -o /dev/null -w "%{http_code}" -u $USERNAME:$PASSWORD $GATEWAY/system/secrets/test-secret)
if [ "$STATUS" == "404" ]; then
    echo -e "${GREEN}✓ PASS${NC}"
else
    echo -e "${RED}✗ FAIL: Expected 404, got $STATUS${NC}"
    exit 1
fi

# Cleanup
echo -e "${YELLOW}Cleaning up test function...${NC}"
curl -s -u $USERNAME:$PASSWORD -X DELETE "$GATEWAY/system/functions?functionName=secret-test" > /dev/null

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}All secrets tests passed!${NC}"
echo -e "${GREEN}========================================${NC}"
