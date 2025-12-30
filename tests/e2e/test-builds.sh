#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"

printf "TEST: Build history endpoints (/system/builds)\n"

list_response="$(curl -s -u "${AUTH_USER}:${AUTH_PASSWORD}" "${GATEWAY}/system/builds")"
if [[ -z "${list_response}" ]]; then
  echo "FAIL: empty build list response"
  exit 1
fi

case "${list_response}" in
  \[*\]) ;;
  *)
    echo "FAIL: expected JSON array"
    echo "${list_response}"
    exit 1
    ;;
esac

status="$(curl -s -o /dev/null -w "%{http_code}" -X DELETE -u "${AUTH_USER}:${AUTH_PASSWORD}" "${GATEWAY}/system/builds")"
if [[ "${status}" != "204" ]]; then
  echo "FAIL: expected 204 on delete, got ${status}"
  exit 1
fi

echo "PASS"
