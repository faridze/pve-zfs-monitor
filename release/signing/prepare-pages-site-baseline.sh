#!/usr/bin/env bash
# Authoritative Pages baseline: live deployed inventory (strict) or one-time gh-pages migration bootstrap.
set -euo pipefail

BASE="${1:-}"
OUT="${2:-}"
GIT_REPO="${3:-}"

if [[ -z "$BASE" || -z "$OUT" ]]; then
  echo "usage: $0 <release-base-url> /path/to/out-dir [git-repo-for-gh-pages]" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
BASE="${BASE%/}"

manifest_probe="$(mktemp)"
http_code="$(
  curl -sS -o "$manifest_probe" -w '%{http_code}' "$BASE/pages-deploy-manifest.txt" || echo "000"
)"
rm -f "$manifest_probe"

if [[ "$http_code" == "200" ]]; then
  PAGES_FETCH_MODE=live PAGES_LIVE_STRICT=1 "$ROOT_SIGNING/fetch-existing-pages-site.sh" "$BASE" "$OUT"
  "$ROOT_SIGNING/verify-pages-deploy-inventory.sh" "$OUT"
  echo "pages baseline from live deployed inventory"
  exit 0
fi

if [[ "$http_code" != "404" ]]; then
  echo "refusing baseline: pages-deploy-manifest.txt HTTP $http_code (need 200 or migration 404)" >&2
  exit 2
fi

if [[ "${PAGES_ALLOW_GH_PAGES_BOOTSTRAP:-}" != "1" ]]; then
  echo "live deploy inventory missing (404) and gh-pages bootstrap not enabled" >&2
  exit 3
fi

staging="$(mktemp -d)"
trap 'rm -rf "$staging"' EXIT

if [[ -n "${PAGES_BOOTSTRAP_SOURCE_DIR:-}" ]]; then
  cp -a "$PAGES_BOOTSTRAP_SOURCE_DIR/." "$staging/"
elif [[ -n "$GIT_REPO" ]]; then
  git -C "$GIT_REPO" fetch origin gh-pages --depth 1 2>/dev/null || true
  git -C "$GIT_REPO" archive origin/gh-pages | tar -x -C "$staging"
else
  echo "gh-pages bootstrap requires GIT_REPO or PAGES_BOOTSTRAP_SOURCE_DIR" >&2
  exit 64
fi

"$ROOT_SIGNING/materialize-public-pages-site.sh" "$staging" "$OUT"
"$ROOT_SIGNING/verify-gh-pages-bootstrap-against-live.sh" "$BASE" "$OUT"
"$ROOT_SIGNING/verify-pages-site-preservation.sh" "$OUT"
echo "pages baseline from verified gh-pages migration bootstrap"
