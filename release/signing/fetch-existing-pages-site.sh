#!/usr/bin/env bash
# Materialize the current public Pages site tree for merge-before-deploy.
# Modes (PAGES_FETCH_MODE):
#   git  — extract refs/heads/gh-pages from a local git repository (default)
#   live — download every path listed in pages-deploy-manifest.txt from HTTPS
set -euo pipefail

BASE="${1:-}"
OUT="${2:-}"
MODE="${PAGES_FETCH_MODE:-git}"
GIT_REPO="${PAGES_GIT_REPO:-}"
BOOTSTRAP="${LAB_BOOTSTRAP_VERSIONS:-1.0.0-rc1}"

if [[ -z "$OUT" ]]; then
  echo "Usage: $0 <release-base-url> /path/to/out-dir" >&2
  echo "  git mode: set PAGES_GIT_REPO to a repo checkout with gh-pages" >&2
  echo "  live mode: set PAGES_FETCH_MODE=live and pass RELEASE_BASE as first arg" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
rm -rf "$OUT"
mkdir -p "$OUT"

if [[ "$MODE" == "git" ]]; then
  if [[ -z "$GIT_REPO" ]]; then
    echo "PAGES_GIT_REPO is required for git fetch mode" >&2
    exit 64
  fi
  git -C "$GIT_REPO" fetch origin gh-pages --depth 1 2>/dev/null || true
  git -C "$GIT_REPO" archive origin/gh-pages | tar -x -C "$OUT"
elif [[ "$MODE" == "live" ]]; then
  if [[ -z "$BASE" ]]; then
    echo "release base URL required for live fetch mode" >&2
    exit 64
  fi
  BASE="${BASE%/}"
  manifest_tmp="$(mktemp)"
  if curl -fsSL "$BASE/pages-deploy-manifest.txt" -o "$manifest_tmp" 2>/dev/null; then
    while IFS= read -r rel || [[ -n "$rel" ]]; do
      rel="$(echo "$rel" | xargs)"
      [[ -z "$rel" ]] && continue
      dest="$OUT/$rel"
      mkdir -p "$(dirname "$dest")"
      curl -fsSL "$BASE/$rel" -o "$dest"
    done < "$manifest_tmp"
    rm -f "$manifest_tmp"
  else
    rm -f "$manifest_tmp"
    echo "pages-deploy-manifest.txt missing; using bootstrap live fetch" >&2
    mkdir -p "$OUT/scripts" "$OUT/lab"
    curl -fsSL "$BASE/index.html" -o "$OUT/index.html"
    curl -fsSL "$BASE/.nojekyll" -o "$OUT/.nojekyll" 2>/dev/null || true
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
    # Preserve optional stable channel tree if present.
    if curl -fsSL "$BASE/stable/.channel-marker" -o "$OUT/stable/.channel-marker" 2>/dev/null; then
      :
    fi
  fi
else
  echo "unknown PAGES_FETCH_MODE=$MODE" >&2
  exit 64
fi

"$ROOT_SIGNING/verify-pages-site-preservation.sh" "$OUT"
echo "fetched Pages site into $OUT (mode=$MODE)"
