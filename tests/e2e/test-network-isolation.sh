#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
USERNAME="${USERNAME:-admin}"
PASSWORD="${PASSWORD:-admin}"
BASE_NETWORK="${FUNCTIONS_NETWORK:-docker-faas-net}"
GATEWAY_CONTAINER_NAME="${GATEWAY_CONTAINER_NAME:-docker-faas-gateway}"

SERVICE_A="net-a-$RANDOM"
SERVICE_B="net-b-$RANDOM"

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

cleanup() {
  curl -s -u "${USERNAME}:${PASSWORD}" -X DELETE "${GATEWAY}/system/functions?functionName=${SERVICE_A}" >/dev/null || true
  curl -s -u "${USERNAME}:${PASSWORD}" -X DELETE "${GATEWAY}/system/functions?functionName=${SERVICE_B}" >/dev/null || true
}

trap cleanup EXIT

echo "Network isolation E2E tests"

deploy() {
  local service="$1"
  local response
  response="$(curl -s -u "${USERNAME}:${PASSWORD}" -X POST "${GATEWAY}/system/functions" \
    -H "Content-Type: application/json" \
    -d "{
      \"service\": \"${service}\",
      \"image\": \"ghcr.io/openfaas/alpine:latest\",
      \"envVars\": {\"fprocess\": \"cat\"}
    }" \
    -w "\n%{http_code}")"
  echo "${response}" | tail -n1
}

status="$(deploy "${SERVICE_A}")"
if [ "${status}" = "202" ]; then
  pass "deployed ${SERVICE_A}"
else
  fail "failed to deploy ${SERVICE_A} (status ${status})"
fi

status="$(deploy "${SERVICE_B}")"
if [ "${status}" = "202" ]; then
  pass "deployed ${SERVICE_B}"
else
  fail "failed to deploy ${SERVICE_B} (status ${status})"
fi

sleep 5

check_network() {
  local network="$1"
  local expected="$2"
  local unexpected="$3"

  local containers
  containers="$(docker network inspect "${network}" --format '{{range $id, $c := .Containers}}{{println $c.Name}}{{end}}')"

  if ! echo "${containers}" | grep -qx "${GATEWAY_CONTAINER_NAME}"; then
    fail "gateway not attached to ${network}"
  fi

  if ! echo "${containers}" | grep -qx "${expected}"; then
    fail "missing ${expected} on ${network}"
  fi

  if echo "${containers}" | grep -qx "${unexpected}"; then
    fail "unexpected ${unexpected} attached to ${network}"
  fi

  pass "network ${network} contains only expected containers"
}

network_a="${BASE_NETWORK}-${SERVICE_A}"
network_b="${BASE_NETWORK}-${SERVICE_B}"

check_network "${network_a}" "${SERVICE_A}-0" "${SERVICE_B}-0"
check_network "${network_b}" "${SERVICE_B}-0" "${SERVICE_A}-0"

echo "Network isolation tests completed"
