#!/usr/bin/env bash
set -euo pipefail

if [ $# -lt 1 ]; then
  echo "Usage: $0 <backup-file>"
  exit 1
fi

BACKUP_FILE="$1"
CONTAINER="${GATEWAY_CONTAINER_NAME:-docker-faas-gateway}"
DB_PATH="${STATE_DB_PATH:-/data/docker-faas.db}"
STOP_CONTAINER="${STOP_CONTAINER:-true}"

if [ ! -f "${BACKUP_FILE}" ]; then
  echo "Backup file not found: ${BACKUP_FILE}"
  exit 1
fi

if [ "${STOP_CONTAINER}" = "true" ]; then
  docker stop "${CONTAINER}" >/dev/null
fi

docker cp "${BACKUP_FILE}" "${CONTAINER}:${DB_PATH}"

if [ "${STOP_CONTAINER}" = "true" ]; then
  docker start "${CONTAINER}" >/dev/null
fi

echo "Restored ${BACKUP_FILE} to ${CONTAINER}:${DB_PATH}"
