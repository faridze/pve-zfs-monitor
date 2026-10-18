#!/usr/bin/env bash
# Maintainer-only: build lab-channel binaries from an immutable Git commit (no signing).
set -euo pipefail

GIT_SHA="${GIT_SHA:-}"
VERSION="${VERSION:-}"
RELEASE_BASE="${RELEASE_BASE:-}"
OUT_DIR="${OUT_DIR:-dist/lab}"

if [[ -z "$GIT_SHA" || -z "$VERSION" || -z "$RELEASE_BASE" ]]; then
  echo "Usage: GIT_SHA=<sha> VERSION=<semver> RELEASE_BASE=https://host/releases OUT_DIR=dist/lab $0" >&2
  exit 64
fi

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

git -C "$ROOT" archive "$GIT_SHA" | tar -x -C "$WORK"
cd "$WORK"

export COMMIT="$GIT_SHA"
export CHANNEL=lab
export TRUSTED=false

if [[ "$OUT_DIR" == /* ]]; then
  ARTIFACT_DIR="$OUT_DIR/$VERSION"
else
  ARTIFACT_DIR="$ROOT/$OUT_DIR/$VERSION"
fi
mkdir -p "$ARTIFACT_DIR"
for pair in linux:amd64 linux:arm64; do
  GOOS="${pair%%:*}"
  GOARCH="${pair##*:}"
  name="pve-zfs-monitor-${GOOS}-${GOARCH}"
  CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" go build -trimpath \
    -ldflags "-s -w \
      -X github.com/faridze/pve-zfs-monitor/internal/release.Version=${VERSION} \
      -X github.com/faridze/pve-zfs-monitor/internal/release.Commit=${GIT_SHA} \
      -X github.com/faridze/pve-zfs-monitor/internal/release.ChannelName=lab" \
    -o "$ARTIFACT_DIR/$name" ./cmd/pve-zfs-monitor
done

python3 "$ROOT/release/signing/assemble-manifest.py" \
  --channel lab \
  --version "$VERSION" \
  --commit "$GIT_SHA" \
  --release-base "$RELEASE_BASE" \
  --artifact-dir "$ARTIFACT_DIR" \
  --output "$ARTIFACT_DIR/manifest.json"

echo "Built unsigned lab bundle at $ARTIFACT_DIR (manifest unsigned)"
