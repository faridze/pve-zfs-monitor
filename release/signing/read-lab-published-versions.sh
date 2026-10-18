#!/usr/bin/env bash
# Print published lab versions (one per line) from canonical inventory files.
set -euo pipefail
SITE="${1:-}"
if [[ -z "$SITE" || ! -d "$SITE" ]]; then
  echo "usage: $0 /path/to/site-root" >&2
  exit 64
fi

for list in "$SITE/lab/published-versions.txt" "$SITE/lab/.published-versions"; do
  if [[ -f "$list" ]]; then
    while IFS= read -r line || [[ -n "$line" ]]; do
      line="${line%%#*}"
      line="$(echo "$line" | xargs)"
      [[ -n "$line" ]] && echo "$line"
    done < "$list"
    exit 0
  fi
done

exit 0
