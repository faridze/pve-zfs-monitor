#!/usr/bin/env bash
# Build unsigned lab bundle at a pinned commit (CI build job — no secrets).
set -euo pipefail

GIT_SHA="${1:-${GIT_SHA:-}}"
VERSION="${2:-${VERSION:-}}"
RELEASE_BASE="${RELEASE_BASE:-https://faridze.github.io/pve-zfs-monitor}"
OUT_DIR="${OUT_DIR:-${RUNNER_TEMP:-/tmp}/lab-build}"

if [[ -z "$GIT_SHA" || -z "$VERSION" ]]; then
  echo "Usage: $0 <git-sha> <version>   (or set GIT_SHA and VERSION)" >&2
  exit 64
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
export GIT_SHA VERSION RELEASE_BASE OUT_DIR
"$ROOT/release/signing/build-lab-artifacts.sh"
echo "out_dir=$OUT_DIR/$VERSION"
