#!/usr/bin/env bash
# Record every file in the Pages site tree for complete fetch on subsequent Actions deploys.
set -euo pipefail

SITE="${1:-}"
if [[ -z "$SITE" || ! -d "$SITE" ]]; then
  echo "usage: $0 /path/to/site-root" >&2
  exit 64
fi

OUT="$SITE/pages-deploy-manifest.txt"
(
  cd "$SITE"
  find . -type f ! -path './pages-deploy-manifest.txt' -printf '%P\n' | LC_ALL=C sort
) > "$OUT"
echo "wrote $(wc -l < "$OUT") paths to pages-deploy-manifest.txt"
