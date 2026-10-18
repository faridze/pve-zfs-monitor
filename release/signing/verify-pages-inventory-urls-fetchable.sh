#!/usr/bin/env bash
# Smoke gate: every path in pages-deploy-manifest.txt must be retrievable over HTTPS.
set -euo pipefail

BASE="${1:-}"
MANIFEST="${2:-}"

if [[ -z "$BASE" || -z "$MANIFEST" ]]; then
  echo "usage: $0 <release-base-url> /path/to/pages-deploy-manifest.txt" >&2
  exit 64
fi

BASE="${BASE%/}"
if [[ ! -f "$MANIFEST" ]]; then
  echo "missing manifest file" >&2
  exit 3
fi

while IFS= read -r rel || [[ -n "$rel" ]]; do
  rel="$(echo "$rel" | xargs)"
  [[ -z "$rel" ]] && continue
  tmp="$(mktemp)"
  code="$(curl -fsSL -o "$tmp" -w '%{http_code}' "$BASE/$rel" || echo "000")"
  rm -f "$tmp"
  if [[ "$code" != "200" ]]; then
    echo "inventory path not fetchable: $rel (HTTP $code)" >&2
    exit 2
  fi
done < "$MANIFEST"

echo "all inventory paths fetchable over HTTPS"
