#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"
DEBUG_BIND_ADDRESS="${DEBUG_BIND_ADDRESS:-127.0.0.1}"

SERVICE="debug-test-$RANDOM"

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

cleanup() {
  curl -s -u "${AUTH_USER}:${AUTH_PASSWORD}" -X DELETE "${GATEWAY}/system/functions?functionName=${SERVICE}" >/dev/null || true
}

trap cleanup EXIT

echo "Debug mode E2E tests"

deploy_response="$(curl -s -u "${AUTH_USER}:${AUTH_PASSWORD}" -X POST "${GATEWAY}/system/functions" \
  -H "Content-Type: application/json" \
  -d "{
    \"service\": \"${SERVICE}\",
    \"image\": \"ghcr.io/openfaas/alpine:latest\",
    \"envVars\": {\"fprocess\": \"cat\"},
    \"debug\": true
  }" \
  -w "\n%{http_code}")"
status="$(echo "${deploy_response}" | tail -n1)"
if [ "${status}" = "202" ]; then
  pass "deployed debug function"
else
  fail "expected 202 deploying debug function, got ${status}"
fi

sleep 5

container="${SERVICE}-0"

binding_40000="$(docker port "${container}" 40000/tcp | head -n 1 || true)"
binding_5678="$(docker port "${container}" 5678/tcp | head -n 1 || true)"

if [ -z "${binding_40000}" ] || [ -z "${binding_5678}" ]; then
  fail "debug ports not mapped"
fi

host_ip_40000="$(echo "${binding_40000}" | awk -F: '{print $1}')"
host_ip_5678="$(echo "${binding_5678}" | awk -F: '{print $1}')"

if [ "${host_ip_40000}" = "${DEBUG_BIND_ADDRESS}" ] && [ "${host_ip_5678}" = "${DEBUG_BIND_ADDRESS}" ]; then
  pass "debug ports bound to ${DEBUG_BIND_ADDRESS}"
else
  fail "debug ports bound to ${host_ip_40000} and ${host_ip_5678}, expected ${DEBUG_BIND_ADDRESS}"
fi

echo "Debug mode tests completed"
