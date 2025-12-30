#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"

printf "TEST: Config endpoint (/system/config)\n"

response="$(curl -s -u "${AUTH_USER}:${AUTH_PASSWORD}" "${GATEWAY}/system/config")"

if [[ -z "${response}" ]]; then
  echo "FAIL: empty config response"
  exit 1
fi

if ! echo "${response}" | grep -q "\"functionsNetwork\""; then
  echo "FAIL: functionsNetwork missing"
  exit 1
fi

if ! echo "${response}" | grep -q "\"authEnabled\""; then
  echo "FAIL: authEnabled missing"
  exit 1
fi

echo "PASS"
