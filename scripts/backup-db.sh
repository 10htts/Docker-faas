#!/usr/bin/env bash
set -euo pipefail

CONTAINER="${GATEWAY_CONTAINER_NAME:-docker-faas-gateway}"
DB_PATH="${STATE_DB_PATH:-/data/docker-faas.db}"
BACKUP_DIR="${BACKUP_DIR:-./backups}"
RETENTION_DAYS="${RETENTION_DAYS:-7}"

timestamp="$(date +%Y%m%d%H%M%S)"
backup_name="backup-${timestamp}.db"
container_backup="/data/${backup_name}"

echo "Creating backup from ${CONTAINER}:${DB_PATH}"
docker exec "${CONTAINER}" sqlite3 "${DB_PATH}" ".backup '${container_backup}'"

mkdir -p "${BACKUP_DIR}"
docker cp "${CONTAINER}:${container_backup}" "${BACKUP_DIR}/${backup_name}"
docker exec "${CONTAINER}" rm -f "${container_backup}"

find "${BACKUP_DIR}" -name "backup-*.db" -mtime +"${RETENTION_DAYS}" -delete
echo "Backup stored at ${BACKUP_DIR}/${backup_name}"
