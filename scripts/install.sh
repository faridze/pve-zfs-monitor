#!/usr/bin/env bash
# pve-zfs-monitor portable installer (no Git/Go on target host).
# Two-step usage (no curl|bash):
#   curl -fsSL -o install.sh "$BASE/scripts/install.sh"
#   sha256sum -c install.sh.sha256   # when maintainer publishes checksum separately
#   sudo bash install.sh --channel lab --version VERSION --release-base https://... --trust-key-file /path/to/key.b64
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

CHANNEL=""
VERSION=""
RELEASE_BASE="${PVE_ZFS_MONITOR_RELEASE_BASE:-}"
TRUST_KEY_FILE="${PVE_ZFS_MONITOR_TRUST_KEY_FILE:-}"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *)
    echo "unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

usage() {
  echo "Usage: sudo $0 --channel stable|lab --version VERSION --release-base URL --trust-key-file PATH" >&2
  exit 64
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --channel) CHANNEL="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --release-base) RELEASE_BASE="$2"; shift 2 ;;
    --trust-key-file) TRUST_KEY_FILE="$2"; shift 2 ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

[[ -n "$CHANNEL" && -n "$VERSION" && -n "$RELEASE_BASE" && -n "$TRUST_KEY_FILE" ]] || usage
[[ "$CHANNEL" == "stable" || "$CHANNEL" == "lab" ]] || usage
[[ -f "$TRUST_KEY_FILE" ]] || { echo "trust key file not found" >&2; exit 3; }
case "$RELEASE_BASE" in
  https://*) ;;
  *) echo "release base must be https://" >&2; exit 3 ;;
esac

command -v python3 >/dev/null || { echo "python3 required on PVE host" >&2; exit 3; }
command -v openssl >/dev/null || { echo "openssl required on PVE host" >&2; exit 3; }

if [[ "$(id -u)" -ne 0 ]]; then
  echo "root privileges required (use sudo)" >&2
  exit 77
fi

TMP="$(mktemp -d /var/tmp/pve-zfs-monitor-install.XXXXXX)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

MANIFEST_URL="${RELEASE_BASE%/}/${CHANNEL}/${VERSION}/manifest.json"
SIG_URL="${RELEASE_BASE%/}/${CHANNEL}/${VERSION}/manifest.json.sig.json"
ARTIFACT_NAME="pve-zfs-monitor-linux-${GOARCH}"
ART_URL="${RELEASE_BASE%/}/${CHANNEL}/${VERSION}/${ARTIFACT_NAME}"

curl -fsSL "$MANIFEST_URL" -o "$TMP/manifest.json"
curl -fsSL "$SIG_URL" -o "$TMP/manifest.json.sig.json"
curl -fsSL "$ART_URL" -o "$TMP/${ARTIFACT_NAME}"

python3 "$SCRIPT_DIR/verify_release.py" \
  --manifest="$TMP/manifest.json" \
  --signature="$TMP/manifest.json.sig.json" \
  --artifact="$TMP/${ARTIFACT_NAME}" \
  --trust-key-file="$TRUST_KEY_FILE" \
  --channel="$CHANNEL" \
  --version="$VERSION" \
  --goarch="$GOARCH"

if command -v pve-zfs-monitor >/dev/null 2>&1; then
  PVE_ZFS_MONITOR_TRUST_KEY_FILE="$TRUST_KEY_FILE" \
    pve-zfs-monitor install-release \
      --manifest="$TMP/manifest.json" \
      --signature="$TMP/manifest.json.sig.json" \
      --binary="$TMP/${ARTIFACT_NAME}" \
      --release-base="$RELEASE_BASE" \
      --channel="$CHANNEL" \
      --version="$VERSION" \
      --trust-key-file="$TRUST_KEY_FILE"
else
  bash "$SCRIPT_DIR/atomic-install.sh" "$TMP/${ARTIFACT_NAME}"
  bash "$SCRIPT_DIR/persist-release-config.sh" "$RELEASE_BASE" "$CHANNEL" "$TRUST_KEY_FILE"
fi

echo "Installed pve-zfs-monitor to /usr/local/bin/pve-zfs-monitor"
