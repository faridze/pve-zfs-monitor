#!/usr/bin/env bash
# Record every public file in the Pages site tree for complete fetch on subsequent Actions deploys.
set -euo pipefail

SITE="${1:-}"
if [[ -z "$SITE" || ! -d "$SITE" ]]; then
  echo "usage: $0 /path/to/site-root" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=pages-public-path-policy.sh
source "$ROOT_SIGNING/pages-public-path-policy.sh"

OUT="$SITE/pages-deploy-manifest.txt"
tmp="$(mktemp)"
while IFS= read -r -d '' f; do
  rel="${f#"$SITE/"}"
  [[ "$rel" == "pages-deploy-manifest.txt" ]] && continue
  if pages_path_is_forbidden "$rel" || ! pages_path_is_public "$rel"; then
    echo "refusing to catalog non-public path: $rel" >&2
    rm -f "$tmp"
    exit 3
  fi
  echo "$rel"
done < <(find "$SITE" -type f -print0) | LC_ALL=C sort > "$tmp"
mv -f "$tmp" "$OUT"
echo "wrote $(wc -l < "$OUT") public paths to pages-deploy-manifest.txt"
