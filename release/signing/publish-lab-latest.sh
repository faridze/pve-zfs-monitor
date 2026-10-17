#!/usr/bin/env bash
# Publish lab/latest metadata alias after a fully verified versioned bundle exists.
# Copies only manifest.json + manifest.json.sig.json; artifact URLs stay version-specific.
set -euo pipefail

VERSION_DIR="${1:-}"
LAB_ROOT="${2:-}"
TRUST_PUB_B64="${3:-}"

if [[ -z "$VERSION_DIR" || -z "$LAB_ROOT" || -z "$TRUST_PUB_B64" ]]; then
  echo "Usage: $0 /path/to/lab/VERSION /path/to/lab-root trusted-signing-key.pub.b64" >&2
  exit 64
fi

ROOT="$(cd "$(dirname "$0")" && pwd)"
"$ROOT/verify-bundle-offline.sh" "$VERSION_DIR" "$TRUST_PUB_B64"

CHANNEL="$(python3 -c "import json;print(json.load(open('$VERSION_DIR/manifest.json'))['channel'])")"
if [[ "$CHANNEL" != "lab" ]]; then
  echo "refusing to publish non-lab manifest to lab/latest" >&2
  exit 3
fi

STAGE="$(mktemp -d "$LAB_ROOT/.latest-publish.XXXXXX")"
cp "$VERSION_DIR/manifest.json" "$VERSION_DIR/manifest.json.sig.json" "$STAGE/"
chmod 0644 "$STAGE"/*

PREV="$LAB_ROOT/latest"
OLD="$LAB_ROOT/.latest-previous"
rm -rf "$OLD"
if [[ -d "$PREV" ]]; then
  mv "$PREV" "$OLD"
fi
mv "$STAGE" "$PREV"
rm -rf "$OLD"
echo "published lab/latest from $(basename "$VERSION_DIR")"
