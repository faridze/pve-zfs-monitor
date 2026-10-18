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
python3 - "$MANIFEST_PATH" "$VERSION_DIR" "$LAB_ROOT" <<'PY'
import json, os, sys

manifest_path, version_dir, lab_root = sys.argv[1:4]
with open(manifest_path, encoding="utf-8") as f:
    doc = json.load(f)
if doc.get("channel") != "lab":
    raise SystemExit("refusing to publish non-lab manifest to lab/latest")
ver = doc.get("version", "")
if not ver:
    raise SystemExit("manifest missing version")
if os.path.basename(os.path.normpath(version_dir)) != ver:
    raise SystemExit(f"version dir basename must match manifest version ({ver})")
for art in doc.get("artifacts", []):
    path = os.path.join(version_dir, art["filename"])
    if not os.path.isfile(path):
        raise SystemExit(f"missing artifact for latest promotion: {path}")
PY

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
latest_ver="$("$ROOT/read-manifest-version.sh" "$PREV/manifest.json")"
target="$LAB_ROOT/$latest_ver"
if [[ ! -d "$target" ]]; then
  echo "lab/latest promoted $latest_ver but lab/$latest_ver is missing" >&2
  exit 3
fi
"$ROOT/verify-bundle-offline.sh" "$target" "$TRUST_PUB_B64"
echo "promoted lab/latest from $latest_ver (local staging only; upload per runbook)"
