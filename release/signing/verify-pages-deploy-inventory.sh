#!/usr/bin/env bash
# Fail closed: deployment inventory must match on-disk public files exactly.
set -euo pipefail

SITE="${1:-}"
if [[ -z "$SITE" || ! -d "$SITE" ]]; then
  echo "usage: $0 /path/to/site-root" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=pages-public-path-policy.sh
source "$ROOT_SIGNING/pages-public-path-policy.sh"

MANIFEST="$SITE/pages-deploy-manifest.txt"
if [[ ! -f "$MANIFEST" ]]; then
  echo "missing pages-deploy-manifest.txt" >&2
  exit 3
fi

declare -A listed=()
while IFS= read -r rel || [[ -n "$rel" ]]; do
  rel="$(echo "$rel" | xargs)"
  [[ -z "$rel" ]] && continue
  if pages_path_is_forbidden "$rel" || ! pages_path_is_public "$rel"; then
    echo "forbidden path in pages-deploy-manifest.txt: $rel" >&2
    exit 3
  fi
  listed["$rel"]=1
  if [[ ! -f "$SITE/$rel" ]]; then
    echo "manifest lists missing file: $rel" >&2
    exit 3
  fi
done < "$MANIFEST"

while IFS= read -r -d '' f; do
  rel="${f#"$SITE/"}"
  [[ "$rel" == "pages-deploy-manifest.txt" ]] && continue
  if pages_path_is_forbidden "$rel" || ! pages_path_is_public "$rel"; then
    echo "unexpected non-public file on disk: $rel" >&2
    exit 3
  fi
  if [[ -z "${listed[$rel]:-}" ]]; then
    echo "public file not listed in pages-deploy-manifest.txt: $rel" >&2
    exit 3
  fi
done < <(find "$SITE" -type f -print0)

"$ROOT_SIGNING/verify-pages-site-preservation.sh" "$SITE"
echo "pages deploy inventory OK"
