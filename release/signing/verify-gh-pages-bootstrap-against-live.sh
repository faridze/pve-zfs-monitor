#!/usr/bin/env bash
# First-migration only: prove gh-pages bootstrap matches the live trusted release snapshot.
set -euo pipefail

BASE="${1:-}"
SITE="${2:-}"

if [[ -z "$BASE" || -z "$SITE" ]]; then
  echo "usage: $0 https://host/site /path/to/bootstrap-site" >&2
  exit 64
fi

BASE="${BASE%/}"
ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"

for rel in index.html scripts/install.sh scripts/install.sh.sha256 scripts/trusted-signing-key.pub.b64; do
  live_tmp="$(mktemp)"
  site_path="$SITE/$rel"
  code="$(curl -fsSL -o "$live_tmp" -w '%{http_code}' "$BASE/$rel" || echo "000")"
  if [[ "$code" != "200" ]]; then
    echo "live asset $rel unavailable (HTTP $code)" >&2
    rm -f "$live_tmp"
    exit 2
  fi
  if [[ ! -f "$site_path" ]]; then
    echo "bootstrap site missing $rel" >&2
    rm -f "$live_tmp"
    exit 3
  fi
  if ! cmp -s "$live_tmp" "$site_path"; then
    echo "bootstrap mismatch for $rel vs live site" >&2
    rm -f "$live_tmp"
    exit 3
  fi
  rm -f "$live_tmp"
done

LIST="$SITE/lab/.published-versions"
if [[ ! -f "$LIST" ]]; then
  # Bootstrap before first Actions inventory: require rc1 on live.
  LIST_CONTENT="1.0.0-rc1"
else
  LIST_CONTENT="$(cat "$LIST")"
fi

while IFS= read -r ver || [[ -n "$ver" ]]; do
  ver="${ver%%#*}"
  ver="$(echo "$ver" | xargs)"
  [[ -z "$ver" ]] && continue
  for f in manifest.json manifest.json.sig.json; do
    rel="lab/$ver/$f"
    live_tmp="$(mktemp)"
    code="$(curl -fsSL -o "$live_tmp" -w '%{http_code}' "$BASE/$rel" || echo "000")"
    if [[ "$code" != "200" ]]; then
      echo "live $rel unavailable (HTTP $code)" >&2
      rm -f "$live_tmp"
      exit 2
    fi
    if ! cmp -s "$live_tmp" "$SITE/$rel"; then
      echo "bootstrap mismatch for $rel vs live site" >&2
      rm -f "$live_tmp"
      exit 3
    fi
    rm -f "$live_tmp"
  done
done <<< "$LIST_CONTENT"

echo "gh-pages bootstrap matches live trusted snapshot"
