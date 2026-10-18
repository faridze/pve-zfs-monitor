#!/usr/bin/env bash
# Fail closed: only HTTP 404 means "not published". Any other outcome aborts.
set -euo pipefail

BASE="${1:-}"
VERSION="${2:-}"

if [[ -z "$BASE" || -z "$VERSION" ]]; then
  echo "usage: $0 https://host/pve-zfs-monitor VERSION" >&2
  exit 64
fi

BASE="${BASE%/}"
url="${BASE}/lab/${VERSION}/manifest.json"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

http_code="$(
  curl -sS -o "$tmp" -w '%{http_code}' "$url" || {
    echo "curl failed for $url" >&2
    exit 2
  }
)"

case "$http_code" in
  404)
    echo "lab version ${VERSION} not published (HTTP 404)"
    exit 0
    ;;
  200)
    echo "lab version ${VERSION} already published at $url" >&2
    exit 1
    ;;
  *)
    echo "unexpected HTTP ${http_code} from $url (refusing to treat as not published)" >&2
    exit 2
    ;;
esac
