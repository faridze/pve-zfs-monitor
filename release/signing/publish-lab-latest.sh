#!/usr/bin/env bash
# Prepare lab/latest metadata locally after a verified versioned bundle exists.
# This updates files under lab/latest/ in place; it does NOT provide atomic remote
# publication on generic HTTPS object stores (see docs/p1d-lab-release-runbook.md).
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

MANIFEST_PATH="$VERSION_DIR/manifest.json"
CHANNEL="$(python3 - "$MANIFEST_PATH" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    print(json.load(f)["channel"])
PY
)"
if [[ "$CHANNEL" != "lab" ]]; then
  echo "refusing to publish non-lab manifest to lab/latest" >&2
  exit 3
fi

if [[ -n "${VERIFY_HTTPS_ORIGIN:-}" ]]; then
  RELEASE_BASE="${RELEASE_BASE:-}"
  if [[ -z "$RELEASE_BASE" ]]; then
    echo "VERIFY_HTTPS_ORIGIN requires RELEASE_BASE" >&2
    exit 64
  fi
  "$ROOT/verify-release-origin.sh" "$MANIFEST_PATH" "$TRUST_PUB_B64" "$RELEASE_BASE"
fi

PREV="$LAB_ROOT/latest"
mkdir -p "$LAB_ROOT"

CANDIDATE="$(mktemp -d "$LAB_ROOT/.latest-candidate.XXXXXX")"
cleanup_candidate() {
  rm -rf "$CANDIDATE"
}
trap cleanup_candidate EXIT

cp "$VERSION_DIR/manifest.json" "$VERSION_DIR/manifest.json.sig.json" "$CANDIDATE/"
chmod 0644 "$CANDIDATE"/*

cmp -s "$CANDIDATE/manifest.json" "$VERSION_DIR/manifest.json"
cmp -s "$CANDIDATE/manifest.json.sig.json" "$VERSION_DIR/manifest.json.sig.json"

if [[ "${PVE_ZFS_MONITOR_PUBLISH_TEST_FAIL:-}" == "before-promote" ]]; then
  echo "test-induced failure before promote" >&2
  exit 1
fi

mkdir -p "$PREV"
MAN_TMP="$(mktemp "$PREV/manifest.json.XXXXXX")"
SIG_TMP="$(mktemp "$PREV/manifest.json.sig.json.XXXXXX")"
cp "$CANDIDATE/manifest.json" "$MAN_TMP"
cp "$CANDIDATE/manifest.json.sig.json" "$SIG_TMP"
chmod 0644 "$MAN_TMP" "$SIG_TMP"
mv -f "$MAN_TMP" "$PREV/manifest.json"
mv -f "$SIG_TMP" "$PREV/manifest.json.sig.json"

trap - EXIT
cleanup_candidate
echo "promoted lab/latest from $(basename "$VERSION_DIR") (local staging only; upload per runbook)"
