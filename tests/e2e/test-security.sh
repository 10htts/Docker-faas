#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin}"

SERVICE="security-test-$RANDOM"
SECRET_NAME="security-secret-$RANDOM"

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

cleanup() {
  curl -s -u "${USERNAME}:${PASSWORD}" -X DELETE "${GATEWAY}/system/functions?functionName=${SERVICE}" >/dev/null || true
  curl -s -u "${USERNAME}:${PASSWORD}" -X DELETE "${GATEWAY}/system/secrets?name=${SECRET_NAME}" >/dev/null || true
}

trap cleanup EXIT

echo "Security E2E tests"

status="$(curl -s -o /dev/null -w "%{http_code}" "${GATEWAY}/system/functions")"
if [ "${status}" = "401" ] || [ "${status}" = "403" ]; then
  pass "auth required for /system/functions"
else
  fail "expected 401/403 without auth, got ${status}"
fi

create_secret="$(curl -s -u "${USERNAME}:${PASSWORD}" -X POST "${GATEWAY}/system/secrets" \
  -H "Content-Type: application/json" \
  -d "{\"name\":\"${SECRET_NAME}\",\"value\":\"secret-value\"}" \
  -w "\n%{http_code}")"
status="$(echo "${create_secret}" | tail -n1)"
if [ "${status}" = "201" ]; then
  pass "created secret"
else
  fail "expected 201 creating secret, got ${status}"
fi

deploy_response="$(curl -s -u "${USERNAME}:${PASSWORD}" -X POST "${GATEWAY}/system/functions" \
  -H "Content-Type: application/json" \
  -d "{
    \"service\": \"${SERVICE}\",
    \"image\": \"ghcr.io/openfaas/alpine:latest\",
    \"envVars\": {\"fprocess\": \"cat /var/openfaas/secrets/${SECRET_NAME}\"},
    \"secrets\": [\"${SECRET_NAME}\"]
  }" \
  -w "\n%{http_code}")"
status="$(echo "${deploy_response}" | tail -n1)"
if [ "${status}" = "202" ]; then
  pass "deployed function with secret"
else
  fail "expected 202 deploying function, got ${status}"
fi

sleep 5

container="${SERVICE}-0"

security_opt="$(docker inspect --format '{{json .HostConfig.SecurityOpt}}' "${container}")"
if echo "${security_opt}" | grep -q "no-new-privileges:true"; then
  pass "no-new-privileges enabled"
else
  fail "no-new-privileges not set"
fi

cap_drop="$(docker inspect --format '{{json .HostConfig.CapDrop}}' "${container}")"
if echo "${cap_drop}" | grep -q "ALL"; then
  pass "CapDrop=ALL enabled"
else
  fail "CapDrop ALL not set"
fi

mounts="$(docker inspect --format '{{json .Mounts}}' "${container}")"
if echo "${mounts}" | grep -q "/var/openfaas/secrets/${SECRET_NAME}" && echo "${mounts}" | grep -q "\"RW\":false"; then
  pass "secret mounted read-only"
else
  fail "secret mount not read-only or missing"
fi

echo "Security tests completed"
