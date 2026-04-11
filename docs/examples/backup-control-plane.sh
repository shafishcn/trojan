#!/usr/bin/env bash
set -euo pipefail

: "${TROJAN_CONTROL_URL:?need TROJAN_CONTROL_URL, e.g. https://control.example.com}"
: "${TROJAN_CONTROL_TOKEN:?need TROJAN_CONTROL_TOKEN}"

BACKUP_DIR="${BACKUP_DIR:-/var/backups/trojan-control}"
STAMP="$(date +%Y%m%d-%H%M%S)"
OUTPUT_FILE="${1:-$BACKUP_DIR/control-backup-${STAMP}.json}"

mkdir -p "$(dirname "$OUTPUT_FILE")"

curl --fail --silent --show-error \
  -H "Authorization: Bearer ${TROJAN_CONTROL_TOKEN}" \
  "${TROJAN_CONTROL_URL%/}/api/control/backup/export" \
  -o "$OUTPUT_FILE"

echo "backup saved to: $OUTPUT_FILE"
