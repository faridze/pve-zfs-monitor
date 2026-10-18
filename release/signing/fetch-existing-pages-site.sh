#!/usr/bin/env bash
# Materialize the current public Pages site tree for merge-before-deploy.
# Modes (PAGES_FETCH_MODE):
#   live — download every path listed in pages-deploy-manifest.txt from HTTPS (strict by default)
#   git  — extract refs/heads/gh-pages from a local git repository (migration/tests only)
set -euo pipefail

BASE="${1:-}"
OUT="${2:-}"
MODE="${PAGES_FETCH_MODE:-live}"
GIT_REPO="${PAGES_GIT_REPO:-}"
STRICT="${PAGES_LIVE_STRICT:-1}"

if [[ -z "$OUT" ]]; then
  echo "Usage: $0 <release-base-url> /path/to/out-dir" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=pages-public-path-policy.sh
source "$ROOT_SIGNING/pages-public-path-policy.sh"

rm -rf "$OUT"
mkdir -p "$OUT"

if [[ "$MODE" == "git" ]]; then
  if [[ -z "$GIT_REPO" ]]; then
    echo "PAGES_GIT_REPO is required for git fetch mode" >&2
    exit 64
  fi
  staging="$(mktemp -d)"
  git -C "$GIT_REPO" fetch origin gh-pages --depth 1 2>/dev/null || true
  git -C "$GIT_REPO" archive origin/gh-pages | tar -x -C "$staging"
  "$ROOT_SIGNING/materialize-public-pages-site.sh" "$staging" "$OUT"
  rm -rf "$staging"
elif [[ "$MODE" == "live" ]]; then
  if [[ -z "$BASE" ]]; then
    echo "release base URL required for live fetch mode" >&2
    exit 64
  fi
  BASE="${BASE%/}"
  manifest_tmp="$(mktemp)"
  http_code="$(
    curl -fsSL -o "$manifest_tmp" -w '%{http_code}' "$BASE/pages-deploy-manifest.txt" || echo "000"
  )"
  if [[ "$http_code" != "200" ]]; then
    rm -f "$manifest_tmp"
    if [[ "$STRICT" == "1" ]]; then
      echo "pages-deploy-manifest.txt must be HTTP 200 (got $http_code)" >&2
      exit 2
    fi
    echo "pages-deploy-manifest.txt missing in non-strict live mode" >&2
    exit 2
  fi

  while IFS= read -r rel || [[ -n "$rel" ]]; do
    rel="$(echo "$rel" | xargs)"
    [[ -z "$rel" ]] && continue
    if pages_path_is_forbidden "$rel" || ! pages_path_is_public "$rel"; then
      echo "forbidden path in live inventory: $rel" >&2
      rm -f "$manifest_tmp"
      exit 3
    fi
    dest="$OUT/$rel"
    mkdir -p "$(dirname "$dest")"
    file_code="$(
      curl -fsSL -o "$dest" -w '%{http_code}' "$BASE/$rel" || echo "000"
    )"
    if [[ "$file_code" != "200" ]]; then
      echo "failed to fetch $rel (HTTP $file_code)" >&2
      rm -f "$manifest_tmp"
      exit 2
    fi
  done < "$manifest_tmp"
  install -m 0644 "$manifest_tmp" "$OUT/pages-deploy-manifest.txt"
  rm -f "$manifest_tmp"
else
  echo "unknown PAGES_FETCH_MODE=$MODE" >&2
  exit 64
fi

if [[ "$MODE" == "live" ]]; then
  "$ROOT_SIGNING/verify-pages-deploy-inventory.sh" "$OUT"
else
  "$ROOT_SIGNING/verify-pages-site-preservation.sh" "$OUT"
fi
echo "fetched Pages site into $OUT (mode=$MODE)"
