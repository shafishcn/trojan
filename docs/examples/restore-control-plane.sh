#!/usr/bin/env bash
set -euo pipefail

: "${TROJAN_CONTROL_URL:?need TROJAN_CONTROL_URL, e.g. https://control.example.com}"
: "${TROJAN_CONTROL_TOKEN:?need TROJAN_CONTROL_TOKEN}"

INPUT_FILE="${1:-}"
if [[ -z "$INPUT_FILE" ]]; then
  echo "usage: $0 /path/to/control-backup.json" >&2
  exit 1
fi

if [[ ! -f "$INPUT_FILE" ]]; then
  echo "backup file not found: $INPUT_FILE" >&2
  exit 1
fi

TMP_PAYLOAD="$(mktemp)"
trap 'rm -f "$TMP_PAYLOAD"' EXIT

python3 - "$INPUT_FILE" "$TMP_PAYLOAD" <<'PY'
import json
import sys

source_path = sys.argv[1]
target_path = sys.argv[2]

with open(source_path, "r", encoding="utf-8") as fh:
    payload = json.load(fh)

if "data" in payload and isinstance(payload["data"], dict) and "backup" in payload["data"]:
    payload = payload["data"]["backup"]

with open(target_path, "w", encoding="utf-8") as fh:
    json.dump(payload, fh, ensure_ascii=False)
PY

curl --fail --silent --show-error \
  -X POST \
  -H "Authorization: Bearer ${TROJAN_CONTROL_TOKEN}" \
  -H "Content-Type: application/json" \
  --data-binary "@${TMP_PAYLOAD}" \
  "${TROJAN_CONTROL_URL%/}/api/control/backup/import"

echo
echo "restore completed from: $INPUT_FILE"
