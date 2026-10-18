#!/usr/bin/env bash
# Assemble a GitHub Pages tree for lab release (signed bundle required).
# Does not push; maintainer reviews OUTPUT_DIR then pushes to gh-pages branch.
set -euo pipefail

VERSION_DIR="${1:-}"
OUTPUT_DIR="${2:-}"
TRUST_PUB_B64="${3:-}"
REPO_ROOT="${4:-}"

if [[ -z "$VERSION_DIR" || -z "$OUTPUT_DIR" || -z "$TRUST_PUB_B64" ]]; then
  echo "Usage: $0 /path/to/lab/VERSION /path/to/gh-pages-output trusted-signing-key.pub.b64 [/path/to/repo-root]" >&2
  exit 64
fi

REPO_ROOT="${REPO_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
VERSION="$(basename "$VERSION_DIR")"
ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"

for f in manifest.json manifest.json.sig.json; do
  [[ -f "$VERSION_DIR/$f" ]] || { echo "missing signed $f in $VERSION_DIR" >&2; exit 3; }
done
for arch in amd64 arm64; do
  [[ -f "$VERSION_DIR/pve-zfs-monitor-linux-$arch" ]] || { echo "missing linux-$arch binary" >&2; exit 3; }
done

"$ROOT_SIGNING/verify-bundle-offline.sh" "$VERSION_DIR" "$TRUST_PUB_B64"

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR/scripts" "$OUTPUT_DIR/lab/$VERSION"

install -m 0644 "$REPO_ROOT/scripts/install.sh" "$OUTPUT_DIR/scripts/install.sh"
cp -a "$VERSION_DIR/manifest.json" "$VERSION_DIR/manifest.json.sig.json" \
  "$VERSION_DIR/pve-zfs-monitor-linux-amd64" "$VERSION_DIR/pve-zfs-monitor-linux-arm64" \
  "$OUTPUT_DIR/lab/$VERSION/"

"$ROOT_SIGNING/publish-lab-latest.sh" "$VERSION_DIR" "$OUTPUT_DIR/lab" "$TRUST_PUB_B64"

sha256sum "$OUTPUT_DIR/scripts/install.sh" | awk '{print $1"  install.sh"}' > "$OUTPUT_DIR/scripts/install.sh.sha256"

python3 - "$OUTPUT_DIR/lab/$VERSION/manifest.json" <<'PY'
import json, sys
base = "https://faridze.github.io/pve-zfs-monitor"
with open(sys.argv[1], encoding="utf-8") as f:
    doc = json.load(f)
for art in doc.get("artifacts", []):
    url = art.get("url", "")
    if not url.startswith(base + "/lab/"):
        raise SystemExit(f"unexpected artifact url: {url}")
print("manifest URLs OK for GitHub Pages")
PY

echo "GitHub Pages tree ready at $OUTPUT_DIR (not pushed)"
