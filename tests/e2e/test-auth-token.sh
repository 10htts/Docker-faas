#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
AUTH_USER="${AUTH_USER:-${DOCKER_FAAS_USER:-admin}}"
AUTH_PASSWORD="${AUTH_PASSWORD:-${DOCKER_FAAS_PASSWORD:-admin}}"

printf "TEST: Auth token login/logout\n"

login_response="$(curl -s -X POST "${GATEWAY}/auth/login" \
  -H "Content-Type: application/json" \
  -d "{\"username\":\"${AUTH_USER}\",\"password\":\"${AUTH_PASSWORD}\"}")"

token="$(printf "%s" "${login_response}" | python -c 'import json,sys; print(json.load(sys.stdin).get("token",""))')"
if [[ -z "${token}" ]]; then
  echo "FAIL: token not returned"
  echo "${login_response}"
  exit 1
fi

status="$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${token}" \
  "${GATEWAY}/system/info")"
if [[ "${status}" != "200" ]]; then
  echo "FAIL: token auth failed (${status})"
  exit 1
fi

logout_status="$(curl -s -o /dev/null -w "%{http_code}" \
  -X POST -H "Authorization: Bearer ${token}" \
  "${GATEWAY}/auth/logout")"
if [[ "${logout_status}" != "204" ]]; then
  echo "FAIL: logout failed (${logout_status})"
  exit 1
fi

status_after="$(curl -s -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${token}" \
  "${GATEWAY}/system/info")"
if [[ "${status_after}" == "200" ]]; then
  echo "FAIL: token still valid after logout"
  exit 1
fi

echo "PASS"
