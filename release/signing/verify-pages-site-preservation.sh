#!/usr/bin/env bash
# Verify a Pages site tree contains required public assets and complete lab bundles.
set -euo pipefail

SITE="${1:-}"
MANIFEST_FILE="${2:-$(cd "$(dirname "$0")" && pwd)/pages-preservation-manifest.txt}"

if [[ -z "$SITE" || ! -d "$SITE" ]]; then
  echo "usage: $0 /path/to/site-root [preservation-manifest.txt]" >&2
  exit 64
fi

ROOT_SIGNING="$(cd "$(dirname "$0")" && pwd)"

while IFS= read -r line || [[ -n "$line" ]]; do
  line="${line%%#*}"
  line="$(echo "$line" | xargs)"
  [[ -z "$line" ]] && continue
  optional=0
  if [[ "$line" == +* ]]; then
    optional=1
    line="${line:1}"
  fi
  path="$SITE/$line"
  if [[ ! -e "$path" ]]; then
    if [[ "$optional" -eq 1 ]]; then
      continue
    fi
    echo "missing required public asset: $line" >&2
    exit 3
  fi
done < "$MANIFEST_FILE"

# Every recorded lab version must have a complete bundle.
LIST="$SITE/lab/.published-versions"
if [[ -f "$LIST" ]]; then
  while IFS= read -r ver || [[ -n "$ver" ]]; do
    ver="${ver%%#*}"
    ver="$(echo "$ver" | xargs)"
    [[ -z "$ver" ]] && continue
    bundle="$SITE/lab/$ver"
    pub="$SITE/scripts/trusted-signing-key.pub.b64"
    if [[ ! -f "$pub" ]]; then
      echo "missing trusted public key for bundle verify" >&2
      exit 3
    fi
    "$ROOT_SIGNING/verify-bundle-offline.sh" "$bundle" "$pub"
  done < "$LIST"
fi

# lab/latest must reference a versioned bundle that exists and is complete.
if [[ -f "$SITE/lab/latest/manifest.json" ]]; then
  latest_ver="$("$ROOT_SIGNING/read-manifest-version.sh" "$SITE/lab/latest/manifest.json")"
  target="$SITE/lab/$latest_ver"
  if [[ ! -d "$target" ]]; then
    echo "lab/latest points to missing lab/$latest_ver" >&2
    exit 3
  fi
  pub="$SITE/scripts/trusted-signing-key.pub.b64"
  "$ROOT_SIGNING/verify-bundle-offline.sh" "$target" "$pub"
  if ! cmp -s "$SITE/lab/latest/manifest.json" "$target/manifest.json"; then
    echo "lab/latest manifest.json does not match lab/$latest_ver" >&2
    exit 3
  fi
fi

echo "pages site preservation checks OK"
