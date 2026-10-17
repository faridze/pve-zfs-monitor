#!/usr/bin/env bash
# Atomically install a verified binary without executing it.
set -euo pipefail
VERIFIED="${1:?verified artifact path}"
TARGET="${2:-/usr/local/bin/pve-zfs-monitor}"
ROLLBACK="${TARGET}.previous"
DIR="$(dirname "$TARGET")"
STAGE="$DIR/.pve-zfs-monitor.staging"

if [[ -L "$TARGET" ]]; then
  echo "refusing to replace symlink at $TARGET" >&2
  exit 3
fi
if [[ "$(id -u)" -ne 0 ]]; then
  echo "root required" >&2
  exit 77
fi

install -m 0755 "$VERIFIED" "$STAGE"
if [[ -f "$TARGET" ]]; then
  install -m 0755 "$TARGET" "${ROLLBACK}.new"
  mv -f "${ROLLBACK}.new" "$ROLLBACK"
fi
mv -f "$STAGE" "$TARGET"
echo "installed to $TARGET"
