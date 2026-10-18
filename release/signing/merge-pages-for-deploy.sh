#!/usr/bin/env bash
# Merge a new signed lab version into an existing GitHub Pages site tree without clobbering prior releases.
set -euo pipefail

EXISTING_SITE="${1:-}"
NEW_VERSION_DIR="${2:-}"
TRUST_PUB_B64="${3:-}"
REPO_ROOT="${4:-}"
EXPECTED_VERSION="${5:-${EXPECTED_LAB_VERSION:-}}"

if [[ -z "$EXISTING_SITE" || -z "$NEW_VERSION_DIR" || -z "$TRUST_PUB_B64" ]]; then
  echo "Usage: $0 /path/to/existing-site /path/to/artifact-bundle trusted-signing-key.pub.b64 [/repo-root] [expected-version]" >&2
  exit 64
fi

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"

"$ROOT_SIGNING/verify-bundle-offline.sh" "$NEW_VERSION_DIR" "$TRUST_PUB_B64"

MANIFEST="$NEW_VERSION_DIR/manifest.json"
VERSION="$("$ROOT_SIGNING/read-manifest-version.sh" "$MANIFEST")"

if [[ -z "$EXPECTED_VERSION" ]]; then
  echo "EXPECTED_LAB_VERSION or argv expected-version is required" >&2
  exit 64
fi
if [[ "$VERSION" != "$EXPECTED_VERSION" ]]; then
  echo "manifest version $VERSION does not match expected $EXPECTED_VERSION" >&2
  exit 3
fi

TARGET_VER="$EXISTING_SITE/lab/$VERSION"
if [[ -f "$TARGET_VER/manifest.json" ]]; then
  if ! cmp -s "$TARGET_VER/manifest.json" "$NEW_VERSION_DIR/manifest.json"; then
    echo "refusing to overwrite existing lab/$VERSION with different manifest" >&2
    exit 3
  fi
  echo "lab/$VERSION unchanged; skipping binary replace"
else
  mkdir -p "$TARGET_VER"
  cp -a "$NEW_VERSION_DIR/manifest.json" "$NEW_VERSION_DIR/manifest.json.sig.json" \
    "$NEW_VERSION_DIR/pve-zfs-monitor-linux-amd64" "$NEW_VERSION_DIR/pve-zfs-monitor-linux-arm64" \
    "$TARGET_VER/"
fi

install -m 0644 "$REPO_ROOT/scripts/install.sh" "$EXISTING_SITE/scripts/install.sh"
sha256sum "$EXISTING_SITE/scripts/install.sh" | awk '{print $1"  install.sh"}' > "$EXISTING_SITE/scripts/install.sh.sha256"
trust_dest="$EXISTING_SITE/scripts/trusted-signing-key.pub.b64"
if [[ "$(realpath "$TRUST_PUB_B64")" != "$(realpath "$trust_dest")" ]]; then
  install -m 0644 "$TRUST_PUB_B64" "$trust_dest"
fi

"$ROOT_SIGNING/verify-bundle-offline.sh" "$TARGET_VER" "$TRUST_PUB_B64"

"$ROOT_SIGNING/publish-lab-latest.sh" "$TARGET_VER" "$EXISTING_SITE/lab" "$TRUST_PUB_B64"

"$ROOT_SIGNING/record-published-lab-version.sh" "$EXISTING_SITE" "$VERSION"

"$ROOT_SIGNING/verify-pages-site-preservation.sh" "$EXISTING_SITE"

echo "merged lab/$VERSION into Pages site (latest promoted)"
