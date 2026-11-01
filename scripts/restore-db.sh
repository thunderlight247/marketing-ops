#!/usr/bin/env bash
# ==============================================================================
# Marketing Ops Platform — PostgreSQL Database Restore Script
# Restores a compressed pg_dump file created by backup-db.sh.
# ==============================================================================
set -euo pipefail

if [ "$#" -lt 1 ]; then
  echo "Usage: $0 <path-to-backup.sql.gz>"
  echo ""
  echo "Available backups in backups/ directory:"
  ls -lh "$(dirname "$0")/../backups"/*.sql.gz 2>/dev/null || echo "No backups found."
  exit 1
fi

BACKUP_FILE="$1"
CONTAINER_NAME="marketing-ops-postgres-1"
DB_USER="LIGHT"
DB_NAME="marketing-ops-platform"

if [ ! -f "${BACKUP_FILE}" ]; then
  echo "ERROR: Backup file '${BACKUP_FILE}' not found!"
  exit 1
fi

# Verify gzip integrity
if ! gzip -t "${BACKUP_FILE}"; then
  echo "ERROR: '${BACKUP_FILE}' is corrupted or not a valid gzip file!"
  exit 1
fi

echo "WARNING: Restoring will overwrite existing data in '${DB_NAME}' inside '${CONTAINER_NAME}'."
read -r -p "Are you sure you want to proceed? (yes/no): " CONFIRM
if [ "${CONFIRM}" != "yes" ]; then
  echo "Restore canceled."
  exit 0
fi

echo "Restoring ${BACKUP_FILE} into ${CONTAINER_NAME}..."
gunzip -c "${BACKUP_FILE}" | docker exec -i "${CONTAINER_NAME}" psql -U "${DB_USER}" -d "${DB_NAME}" --single-transaction

echo "SUCCESS: Database restored from ${BACKUP_FILE}."
