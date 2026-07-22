#!/usr/bin/env bash
# End-to-end OpenFaaS-contract smoke test against a containerized gateway using
# the PINNED official faas-cli (0.18.0). Exercises: login, deploy, list,
# describe, sync invoke, async invoke, scale, secrets CRUD, logs, namespaces,
# info/version, health, and remove — the mandatory surface a standard client
# uses with NO extension awareness (redteam RT-205/RT-207 evidence).
#
# The default run is isolated even on a shared Docker daemon: it generates
# unique project/container/network/volume/image/function names, binds random
# localhost ports, and enables strict container ownership.
#
# Usage: bash scripts/smoke-openfaas-cli.sh [--keep]
#   --keep  leave the isolated compose stack running afterwards.
#
# Optional overrides: SMOKE_ID, SMOKE_GATEWAY_URL, SMOKE_GATEWAY_HOST_PORT,
# SMOKE_METRICS_HOST_PORT. SMOKE_GATEWAY_URL is intended only for harness
# debugging; the script still starts and later tears down its own isolated stack.
#
# Requires: docker (daemon running), docker compose, faas-cli 0.18.0, curl.
set -uo pipefail

cd "$(dirname "$0")/.."

KEEP=${1:-}
SMOKE_ID="${SMOKE_ID:-dfsmoke-$(date +%s)-$$}"
SMOKE_ID=$(printf '%s' "$SMOKE_ID" | tr '[:upper:]_' '[:lower:]-' | tr -cd 'a-z0-9-')
if [ -z "$SMOKE_ID" ]; then
  echo "SMOKE_ID must contain at least one alphanumeric character" >&2
  exit 2
fi

export COMPOSE_PROJECT_NAME="${SMOKE_COMPOSE_PROJECT_NAME:-$SMOKE_ID}"
export GATEWAY_IMAGE="${SMOKE_GATEWAY_IMAGE:-docker-faas/gateway-smoke:$SMOKE_ID}"
export GATEWAY_CONTAINER_NAME="${SMOKE_GATEWAY_CONTAINER_NAME:-$SMOKE_ID-gateway}"
export GATEWAY_NETWORK_NAME="${SMOKE_GATEWAY_NETWORK_NAME:-$SMOKE_ID-gateway-net}"
export FUNCTIONS_NETWORK="${SMOKE_FUNCTIONS_NETWORK:-$SMOKE_ID-fn-net}"
export FAAS_DATA_VOLUME="${SMOKE_DATA_VOLUME:-$SMOKE_ID-data}"
export FAAS_SECRETS_VOLUME="${SMOKE_SECRETS_VOLUME:-$SMOKE_ID-secrets}"
export GATEWAY_HOST_BIND="${SMOKE_GATEWAY_HOST_BIND:-127.0.0.1}"
export METRICS_HOST_BIND="${SMOKE_METRICS_HOST_BIND:-127.0.0.1}"
export GATEWAY_HOST_PORT="${SMOKE_GATEWAY_HOST_PORT:-0}"
export METRICS_HOST_PORT="${SMOKE_METRICS_HOST_PORT:-0}"
export FAAS_STRICT_CONTAINER_OWNERSHIP=true

FN="${SMOKE_FUNCTION_NAME:-$SMOKE_ID-figlet}"
IMAGE=ghcr.io/openfaas/figlet:latest
PASS_COUNT=0
FAIL_COUNT=0

cleanup() {
  status=$?
  trap - EXIT INT TERM
  if [ "$KEEP" != "--keep" ]; then
    # Function containers and networks are daemon resources, not Compose
    # children. The generated ownership identity makes this cleanup exact.
    for container_id in $(docker ps -aq --filter "label=com.docker-faas.gateway=$FUNCTIONS_NETWORK" 2>/dev/null); do
      docker rm -f "$container_id" >/dev/null 2>&1 || true
    done
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
    docker network rm "$FUNCTIONS_NETWORK-$FN" "$FUNCTIONS_NETWORK" "$GATEWAY_NETWORK_NAME" >/dev/null 2>&1 || true
    docker volume rm -f "$FAAS_DATA_VOLUME" "$FAAS_SECRETS_VOLUME" >/dev/null 2>&1 || true
    docker image rm "$GATEWAY_IMAGE" >/dev/null 2>&1 || true
  else
    printf '\nIsolated stack kept: project=%s gateway=%s function=%s\n' "$COMPOSE_PROJECT_NAME" "$GATEWAY_CONTAINER_NAME" "$FN"
  fi
  exit "$status"
}
trap cleanup EXIT INT TERM

step() { printf '\n=== %s ===\n' "$*"; }
ok()   { PASS_COUNT=$((PASS_COUNT+1)); printf 'PASS: %s\n' "$*"; }
fail() { FAIL_COUNT=$((FAIL_COUNT+1)); printf 'FAIL: %s\n' "$*"; }
check() { # check <desc> <cmd...>
  local desc="$1"; shift
  if "$@"; then ok "$desc"; else fail "$desc (cmd: $*)"; fi
}

step "faas-cli version (pin evidence)"
faas-cli version --short-version || true

step "compose up --build"
docker compose up -d --build || { echo "compose up failed"; exit 1; }

if [ -n "${SMOKE_GATEWAY_URL:-}" ]; then
  GATEWAY_URL="$SMOKE_GATEWAY_URL"
else
  gateway_binding=$(docker compose port gateway 8080) || { echo "unable to resolve gateway port"; exit 1; }
  GATEWAY_URL="http://127.0.0.1:${gateway_binding##*:}"
fi
echo "isolated gateway: $GATEWAY_URL (project=$COMPOSE_PROJECT_NAME, owner=$FUNCTIONS_NETWORK)"

step "wait for /healthz"
deadline=$((SECONDS+180))
until curl -fsS "$GATEWAY_URL/healthz" >/dev/null 2>&1; do
  if [ $SECONDS -gt $deadline ]; then echo "gateway never became healthy"; docker compose logs --tail 50 gateway; exit 1; fi
  sleep 2
done
ok "gateway healthy"

export OPENFAAS_URL="$GATEWAY_URL"

step "faas-cli login"
check "login" bash -c 'echo admin | faas-cli login -u admin --password-stdin'

step "deploy ($IMAGE)"
check "deploy" faas-cli deploy --image "$IMAGE" --name "$FN" --label com.openfaas.scale.min=1 --label com.openfaas.scale.max=3

step "list"
check "list shows function" bash -c "faas-cli list | grep -q $FN"

step "describe (GET /system/function/{name})"
check "describe" faas-cli describe "$FN"

step "sync invoke"
check "invoke returns figlet output" bash -c "echo hi | faas-cli invoke $FN | grep -q '_'"

step "async invoke (202 + X-Call-Id)"
async_headers=$(echo hi | curl -sS -D - -o /dev/null -X POST -u admin:admin --data-binary @- "$GATEWAY_URL/async-function/$FN")
echo "$async_headers" | head -8
check "async returns 202" bash -c "echo '$async_headers' | head -1 | grep -q 202"
check "async returns X-Call-Id" bash -c "echo '$async_headers' | grep -qi 'x-call-id'"

step "scale to 2 (POST /system/scale-function/{name})"
check "scale accepted" bash -c "curl -sS -o /dev/null -w '%{http_code}' -X POST -u admin:admin -H 'Content-Type: application/json' -d '{\"serviceName\":\"$FN\",\"replicas\":2}' $GATEWAY_URL/system/scale-function/$FN | grep -qE '20(0|2)'"
sleep 3
check "describe shows replicas" bash -c "faas-cli describe $FN | grep -iq replica"

step "secrets CRUD"
check "secret create" bash -c "echo -n s3cr3t | faas-cli secret create smoke-secret"
check "secret list"   bash -c "faas-cli secret list | grep -q smoke-secret"
check "secret update" bash -c "echo -n s3cr3t2 | faas-cli secret update smoke-secret"
check "secret remove" faas-cli secret remove smoke-secret

step "logs (GET /system/logs NDJSON)"
check "faas-cli logs parses" faas-cli logs "$FN" --tail=false --lines 20

step "namespaces"
check "faas-cli namespaces" faas-cli namespaces

step "gateway info via faas-cli version"
check "gateway info parsed" bash -c "faas-cli version | grep -iq 'provider'"

step "raw contract checks"
check "GET /system/function/$FN 200" bash -c "curl -sS -o /dev/null -w '%{http_code}' -u admin:admin $GATEWAY_URL/system/function/$FN | grep -q 200"
check "GET /system/function/unknown 404" bash -c "curl -sS -o /dev/null -w '%{http_code}' -u admin:admin $GATEWAY_URL/system/function/does-not-exist | grep -q 404"
check "GET /system/namespaces 200" bash -c "curl -sS -o /dev/null -w '%{http_code}' -u admin:admin $GATEWAY_URL/system/namespaces | grep -q 200"
check "invoke with namespace suffix" bash -c "echo hi | curl -sS -o /dev/null -w '%{http_code}' -X POST -u admin:admin --data-binary @- $GATEWAY_URL/function/$FN.openfaas-fn | grep -q 200"
check "extensions off: capabilities endpoint additive" bash -c "curl -sS -o /dev/null -w '%{http_code}' -u admin:admin $GATEWAY_URL/system/scale/capabilities | grep -q 200"
check "activity-lease unsigned rejected (401/503)" bash -c "curl -sS -o /dev/null -w '%{http_code}' -X POST -u admin:admin -H 'Content-Type: application/json' -d '{\"contract_version\":\"1.0.0\",\"function\":\"$FN\"}' $GATEWAY_URL/system/scale/activity-lease | grep -qE '(401|503)'"

step "remove"
check "remove" faas-cli remove "$FN"
sleep 2
check "list no longer shows function" bash -c "! faas-cli list | grep -q $FN"

step "summary"
echo "PASS=$PASS_COUNT FAIL=$FAIL_COUNT"

[ "$FAIL_COUNT" -eq 0 ]
