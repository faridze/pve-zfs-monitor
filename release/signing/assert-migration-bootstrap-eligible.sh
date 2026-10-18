#!/usr/bin/env bash
# gh-pages bootstrap is allowed only when live site is still at the original signed rc1 baseline.
set -euo pipefail

BASE="${1:-}"
BASELINE="${MIGRATION_BASELINE_VERSION:-1.0.0-rc1}"

if [[ -z "$BASE" ]]; then
  echo "usage: $0 <release-base-url>" >&2
  exit 64
fi

BASE="${BASE%/}"
ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"

fetch_live() {
  local rel="$1"
  local out="$2"
  local code
  code="$(curl -fsSL -o "$out" -w '%{http_code}' "$BASE/$rel" || echo "000")"
  if [[ "$code" != "200" ]]; then
    echo "live $rel unavailable (HTTP $code)" >&2
    return 2
  fi
  return 0
}

latest_man="$(mktemp)"
latest_sig="$(mktemp)"
trap 'rm -f "$latest_man" "$latest_sig"' EXIT

fetch_live "lab/latest/manifest.json" "$latest_man" || exit 2
fetch_live "lab/latest/manifest.json.sig.json" "$latest_sig" || exit 2

latest_ver="$("$ROOT_SIGNING/read-manifest-version.sh" "$latest_man")"
if [[ "$latest_ver" != "$BASELINE" ]]; then
  echo "refusing gh-pages bootstrap: live lab/latest is $latest_ver (need $BASELINE only)" >&2
  exit 3
fi

base_man="$(mktemp)"
base_sig="$(mktemp)"
trap 'rm -f "$latest_man" "$latest_sig" "$base_man" "$base_sig"' EXIT

fetch_live "lab/$BASELINE/manifest.json" "$base_man" || exit 2
fetch_live "lab/$BASELINE/manifest.json.sig.json" "$base_sig" || exit 2

if ! cmp -s "$latest_man" "$base_man"; then
  echo "live lab/latest manifest does not match lab/$BASELINE" >&2
  exit 3
fi
if ! cmp -s "$latest_sig" "$base_sig"; then
  echo "live lab/latest signature does not match lab/$BASELINE" >&2
  exit 3
fi

# Block bootstrap if any later immutable lab release is already on the live site.
for probe in 1.0.0-rc2 1.0.0-rc3 1.0.0-rc4 1.0.0 1.0.1; do
  [[ "$probe" == "$BASELINE" ]] && continue
  tmp="$(mktemp)"
  code="$(curl -sS -o "$tmp" -w '%{http_code}' "$BASE/lab/$probe/manifest.json" || echo "000")"
  rm -f "$tmp"
  if [[ "$code" == "200" ]]; then
    echo "refusing gh-pages bootstrap: live already has lab/$probe" >&2
    exit 3
  fi
  if [[ "$code" != "404" ]]; then
    echo "unexpected HTTP $code probing lab/$probe/manifest.json" >&2
    exit 2
  fi
done

echo "live site eligible for one-time gh-pages migration bootstrap ($BASELINE)"
