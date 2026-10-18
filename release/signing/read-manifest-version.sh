#!/usr/bin/env bash
# Print the version field from a release manifest.json.
set -euo pipefail
MANIFEST="${1:-}"
if [[ -z "$MANIFEST" || ! -f "$MANIFEST" ]]; then
  echo "usage: $0 /path/to/manifest.json" >&2
  exit 64
fi
python3 - "$MANIFEST" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    doc = json.load(f)
ver = doc.get("version", "")
if not ver:
    raise SystemExit("manifest missing version")
print(ver)
PY
