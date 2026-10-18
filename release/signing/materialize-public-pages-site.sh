#!/usr/bin/env bash
# Copy only allowlisted public web assets into a clean deployment tree (no .git or repo internals).
set -euo pipefail

SRC="${1:-}"
DST="${2:-}"

if [[ -z "$SRC" || -z "$DST" ]]; then
  echo "usage: $0 /path/to/dirty-site /path/to/clean-out" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=pages-public-path-policy.sh
source "$ROOT_SIGNING/pages-public-path-policy.sh"

rm -rf "$DST"
mkdir -p "$DST"

while IFS= read -r -d '' f; do
  rel="${f#"$SRC/"}"
  pages_path_is_public "$rel" || continue
  mkdir -p "$DST/$(dirname "$rel")"
  cp -a "$f" "$DST/$rel"
done < <(find "$SRC" -type f -print0)

while IFS= read -r -d '' f; do
  rel="${f#"$DST/"}"
  if pages_path_is_forbidden "$rel" || ! pages_path_is_public "$rel"; then
    echo "forbidden path in clean deployment tree: $rel" >&2
    exit 3
  fi
  if [[ -L "$f" ]]; then
    echo "symlinks are not allowed in deployment tree: $rel" >&2
    exit 3
  fi
done < <(find "$DST" -type f -print0)

echo "materialized clean public Pages tree at $DST"
