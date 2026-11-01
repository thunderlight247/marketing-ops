#!/usr/bin/env bash
# ==============================================================================
# Marketing Ops Platform — PostgreSQL Automated Backup Script
# Creates a compressed, timestamped pg_dump of the database and prunes old dumps.
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
BACKUP_DIR="${ROOT_DIR}/backups"
LOG_FILE="${BACKUP_DIR}/backup.log"

CONTAINER_NAME="marketing-ops-postgres-1"
DB_USER="LIGHT"
DB_NAME="marketing-ops-platform"
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-7}"

mkdir -p "${BACKUP_DIR}"

log() {
  local msg="[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] $*"
  echo "${msg}"
  echo "${msg}" >> "${LOG_FILE}"
}

log "INFO: Starting database backup for '${DB_NAME}'..."

# Verify docker container is running
if ! docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
  log "ERROR: Container '${CONTAINER_NAME}' is not running! Backup aborted."
  exit 1
fi

TIMESTAMP="$(date +'%Y%m%d_%H%M%S')"
BACKUP_FILE="${BACKUP_DIR}/marketing_ops_${TIMESTAMP}.sql.gz"
TEMP_FILE="${BACKUP_FILE}.tmp"

# Dump and compress
if docker exec "${CONTAINER_NAME}" pg_dump -U "${DB_USER}" -d "${DB_NAME}" --no-owner --no-privileges | gzip -9 > "${TEMP_FILE}"; then
  # Verify archive integrity
  if gzip -t "${TEMP_FILE}"; then
    mv "${TEMP_FILE}" "${BACKUP_FILE}"
    FILE_SIZE="$(du -h "${BACKUP_FILE}" | cut -f1)"
    log "SUCCESS: Backup completed -> ${BACKUP_FILE} (${FILE_SIZE})"
  else
    rm -f "${TEMP_FILE}"
    log "ERROR: Backup archive failed gzip integrity check!"
    exit 1
  fi
else
  rm -f "${TEMP_FILE}"
  log "ERROR: pg_dump command failed!"
  exit 1
fi

# Prune backups older than retention days
log "INFO: Pruning backups older than ${RETENTION_DAYS} days..."
PRUNED_COUNT=0
while IFS= read -r old_backup; do
  if [ -n "${old_backup}" ]; then
    rm -f "${old_backup}"
    log "INFO: Removed old backup: ${old_backup}"
    PRUNED_COUNT=$((PRUNED_COUNT + 1))
  fi
done < <(find "${BACKUP_DIR}" -maxdepth 1 -name "marketing_ops_*.sql.gz" -mtime +"${RETENTION_DAYS}")

log "INFO: Backup finished successfully. Pruned ${PRUNED_COUNT} old backup(s)."
