#!/usr/bin/env bash
# Download the currently published GitHub Pages site tree for merge-before-deploy.
# Uses HTTPS (live site). Bootstrap LAB_BOOTSTRAP_VERSIONS when .published-versions is absent.
set -euo pipefail

BASE="${1:-}"
OUT="${2:-}"
BOOTSTRAP="${LAB_BOOTSTRAP_VERSIONS:-1.0.0-rc1}"

if [[ -z "$BASE" || -z "$OUT" ]]; then
  echo "Usage: $0 https://faridze.github.io/pve-zfs-monitor /path/to/out-dir" >&2
  exit 64
fi

BASE="${BASE%/}"
mkdir -p "$OUT/scripts" "$OUT/lab"

curl -fsSL "$BASE/index.html" -o "$OUT/index.html"

for f in install.sh install.sh.sha256 trusted-signing-key.pub.b64; do
  curl -fsSL "$BASE/scripts/$f" -o "$OUT/scripts/$f"
done

versions=()
if curl -fsSL "$BASE/lab/.published-versions" -o "$OUT/lab/.published-versions" 2>/dev/null; then
  while IFS= read -r line || [[ -n "$line" ]]; do
    line="${line%%#*}"
    line="$(echo "$line" | xargs)"
    [[ -n "$line" ]] && versions+=("$line")
  done < "$OUT/lab/.published-versions"
else
  for ver in ${BOOTSTRAP//,/ }; do
    [[ -n "$ver" ]] && versions+=("$ver")
  done
fi

for ver in "${versions[@]}"; do
  dest="$OUT/lab/$ver"
  mkdir -p "$dest"
  for f in manifest.json manifest.json.sig.json pve-zfs-monitor-linux-amd64 pve-zfs-monitor-linux-arm64; do
    curl -fsSL "$BASE/lab/$ver/$f" -o "$dest/$f"
  done
done

mkdir -p "$OUT/lab/latest"
for f in manifest.json manifest.json.sig.json; do
  curl -fsSL "$BASE/lab/latest/$f" -o "$OUT/lab/latest/$f"
done

echo "fetched Pages site into $OUT (${#versions[@]} version dir(s))"
