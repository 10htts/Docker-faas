#!/usr/bin/env bash
set -euo pipefail

GATEWAY="${GATEWAY:-http://localhost:8080}"
USERNAME="${AUTH_USER:-admin}"
PASSWORD="${AUTH_PASSWORD:-admin}"
FUNCTION_NAME="${FAAS_FUNCTION_NAME:-cli-workflow}"
IMAGE="${FAAS_IMAGE:-ghcr.io/openfaas/alpine:latest}"
FPROCESS="${FAAS_FPROCESS:-cat}"

cleanup() {
  faas-cli remove "$FUNCTION_NAME" --gateway "$GATEWAY" >/dev/null 2>&1 || true
}

trap cleanup EXIT

if ! command -v faas-cli >/dev/null 2>&1; then
  echo "faas-cli not found. Please install it first."
  exit 1
fi

if ! command -v curl >/dev/null 2>&1; then
  echo "curl not found. Please install it first."
  exit 1
fi

echo "Logging in..."
echo "$PASSWORD" | faas-cli login --gateway "$GATEWAY" --username "$USERNAME" --password-stdin >/dev/null

echo "Deploying function..."
faas-cli deploy \
  --gateway "$GATEWAY" \
  --image "$IMAGE" \
  --name "$FUNCTION_NAME" \
  --env "fprocess=$FPROCESS" >/dev/null

echo "Invoking function..."
result=$(echo "hello" | faas-cli invoke "$FUNCTION_NAME" --gateway "$GATEWAY")
if [ "$result" != "hello" ]; then
  echo "Unexpected invocation response: $result"
  exit 1
fi

echo "Scaling function..."
if faas-cli scale --help >/dev/null 2>&1; then
  faas-cli scale "$FUNCTION_NAME" --gateway "$GATEWAY" --replicas 2 >/dev/null
else
  echo "faas-cli scale not available; using API scale endpoint"
  status=$(curl -s -o /dev/null -w "%{http_code}" \
    -u "$USERNAME:$PASSWORD" \
    -H "Content-Type: application/json" \
    -d "{\"serviceName\":\"$FUNCTION_NAME\",\"replicas\":2}" \
    "$GATEWAY/system/scale-function/$FUNCTION_NAME")
  if [ "$status" != "202" ]; then
    echo "Scale endpoint failed with status $status"
    exit 1
  fi
fi

echo "Listing functions..."
faas-cli list --gateway "$GATEWAY" | grep -q "$FUNCTION_NAME"

echo "Removing function..."
faas-cli remove "$FUNCTION_NAME" --gateway "$GATEWAY" >/dev/null

echo "faas-cli workflow test passed."
