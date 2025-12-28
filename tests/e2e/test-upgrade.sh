#!/usr/bin/env bash
set -euo pipefail

IMAGE="${GATEWAY_IMAGE:-docker-faas/gateway:latest}"
TEMP_DIR_BASE="${TMPDIR:-/tmp}"
GATEWAY_CONTAINER="docker-faas-upgrade-test"
GATEWAY_PORT="${UPGRADE_GATEWAY_PORT:-8081}"
METRICS_PORT="${UPGRADE_METRICS_PORT:-9091}"
DOCKER_SOCK="/var/run/docker.sock"

case "$(uname -s)" in
  MINGW*|MSYS*|CYGWIN*)
    if [ -n "${TEMP:-}" ]; then
      if command -v cygpath >/dev/null 2>&1; then
        TEMP_DIR_BASE="$(cygpath -u "$TEMP")"
      else
        TEMP_DIR_BASE="${TEMP}"
      fi
    fi
    if [[ "${TEMP_DIR_BASE}" == /tmp* ]]; then
      if command -v cygpath >/dev/null 2>&1; then
        if [ -n "${LOCALAPPDATA:-}" ]; then
          TEMP_DIR_BASE="$(cygpath -u "$LOCALAPPDATA")/Temp"
        elif [ -n "${USERPROFILE:-}" ]; then
          TEMP_DIR_BASE="$(cygpath -u "$USERPROFILE")/AppData/Local/Temp"
        fi
      fi
    fi
    DOCKER_SOCK="//./pipe/docker_engine"
    export MSYS2_ARG_CONV_EXCL="*"
    ;;
esac

TEMP_DIR="$(mktemp -d -p "${TEMP_DIR_BASE}" docker-faas-upgrade-XXXXXX 2>/dev/null || mktemp -d)"
DB_PATH="${TEMP_DIR}/docker-faas.db"
SQLITE_DB_PATH="${DB_PATH}"
if command -v cygpath >/dev/null 2>&1; then
  SQLITE_DB_PATH="$(cygpath -w "$DB_PATH")"
fi
DOCKER_HOST_PATH="${TEMP_DIR}"
if command -v cygpath >/dev/null 2>&1; then
  DOCKER_HOST_PATH="$(cygpath -m "$TEMP_DIR")"
fi

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

cleanup() {
  docker rm -f "${GATEWAY_CONTAINER}" >/dev/null 2>&1 || true
  rm -rf "${TEMP_DIR}"
}

trap cleanup EXIT

echo "Upgrade/migration E2E tests"

if ! command -v sqlite3 >/dev/null 2>&1; then
  fail "sqlite3 is required for this test"
fi

if ! docker image inspect "${IMAGE}" >/dev/null 2>&1; then
  fail "gateway image ${IMAGE} not found (build or docker-compose up first)"
fi

sqlite3 "${SQLITE_DB_PATH}" <<'SQL'
CREATE TABLE IF NOT EXISTS functions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT UNIQUE NOT NULL,
  image TEXT NOT NULL,
  env_process TEXT,
  env_vars TEXT,
  labels TEXT,
  secrets TEXT,
  network TEXT NOT NULL,
  replicas INTEGER NOT NULL DEFAULT 1,
  limits TEXT,
  requests TEXT,
  read_only BOOLEAN NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL,
  updated_at TIMESTAMP NOT NULL
);
SQL

docker run -d --rm \
  --name "${GATEWAY_CONTAINER}" \
  -p "${GATEWAY_PORT}:8080" \
  -p "${METRICS_PORT}:9090" \
  -v "${DOCKER_SOCK}:/var/run/docker.sock" \
  -v "${DOCKER_HOST_PATH}:/data" \
  -e STATE_DB_PATH=/data/docker-faas.db \
  -e GATEWAY_PORT=8080 \
  -e METRICS_PORT=9090 \
  -e AUTH_ENABLED=false \
  -e FUNCTIONS_NETWORK=docker-faas-net \
  -e DEBUG_BIND_ADDRESS=127.0.0.1 \
  "${IMAGE}" >/dev/null

sleep 5

version="$(sqlite3 "${SQLITE_DB_PATH}" <<'SQL'
SELECT COALESCE(MAX(version), 0) FROM schema_migrations;
SQL
)"
if [ "${version}" -ge 2 ]; then
  pass "migrations applied (version ${version})"
else
  fail "expected schema_migrations version >= 2, got ${version}"
fi

echo "Upgrade tests completed"
