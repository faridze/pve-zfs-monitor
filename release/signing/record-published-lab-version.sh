#!/usr/bin/env bash
# Append an immutable lab version to lab/.published-versions (idempotent).
set -euo pipefail

SITE_ROOT="${1:-}"
VERSION="${2:-}"

if [[ -z "$SITE_ROOT" || -z "$VERSION" ]]; then
  echo "Usage: $0 /path/to/site-root VERSION" >&2
  exit 64
fi

mkdir -p "$SITE_ROOT/lab"
for LIST in "$SITE_ROOT/lab/published-versions.txt" "$SITE_ROOT/lab/.published-versions"; do
  touch "$LIST"
  if ! grep -qxF "$VERSION" "$LIST"; then
    echo "$VERSION" >> "$LIST"
  fi
done
