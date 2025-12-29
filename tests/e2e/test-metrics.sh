#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"

printf "TEST: Metrics endpoint (/system/metrics)\n"

response="$(curl -s -u "${AUTH_USER}:${AUTH_PASSWORD}" "${GATEWAY}/system/metrics")"

if [[ -z "${response}" ]]; then
  echo "FAIL: empty metrics response"
  exit 1
fi

if ! echo "${response}" | grep -q "^gateway_http_requests_total"; then
  echo "FAIL: gateway_http_requests_total not found"
  exit 1
fi

if ! echo "${response}" | grep -q "^function_invocations_total"; then
  echo "FAIL: function_invocations_total not found"
  exit 1
fi

echo "PASS"
