#!/usr/bin/env bash
# Self-contained pve-zfs-monitor installer (no Git/Go). All verifier/install logic is embedded in this file.
# Out-of-band: verify this script's SHA-256 (or maintainer signature) before running as root.
#
#   curl --proto '=https' --tlsv1.2 --proto-redir '=https' -fsSL -o install.sh "$BASE/scripts/install.sh"
#   sha256sum -c install.sh.sha256    # from maintainer, not the release host alone
#   sudo INSTALLER_SHA256=<expected> bash install.sh --channel lab --version VER \
#     --release-base https://... --trust-key-file /path/to/reviewed-key.b64
set -euo pipefail

LOCK_PATH="${PVE_ZFS_MONITOR_LOCK_PATH:-/run/lock/pve-zfs-monitor.lock}"
TARGET="${PVE_ZFS_MONITOR_INSTALL_TARGET:-/usr/local/bin/pve-zfs-monitor}"
ROLLBACK="${TARGET}.previous"
CONF_DIR="${PVE_ZFS_MONITOR_CONFIG_DIR:-/etc/pve-zfs-monitor}"
TEST_INSTALL="${PVE_ZFS_MONITOR_TEST_INSTALL:-}"

CHANNEL=""
VERSION=""
RELEASE_BASE="${PVE_ZFS_MONITOR_RELEASE_BASE:-}"
TRUST_KEY_FILE="${PVE_ZFS_MONITOR_TRUST_KEY_FILE:-}"
INSTALLER_SHA256="${INSTALLER_SHA256:-}"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

usage() {
  echo "Usage: sudo INSTALLER_SHA256=<hex> bash $0 --channel stable|lab --version VER --release-base URL --trust-key-file PATH" >&2
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
case "$RELEASE_BASE" in https://*) ;; *) echo "release base must be https://" >&2; exit 3 ;; esac

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 3; }
command -v openssl >/dev/null || { echo "openssl required" >&2; exit 3; }
command -v curl >/dev/null || { echo "curl required" >&2; exit 3; }
command -v flock >/dev/null || { echo "flock required" >&2; exit 3; }

if [[ "$TEST_INSTALL" != "1" ]] && [[ "$(id -u)" -ne 0 ]]; then
  echo "root privileges required (use sudo)" >&2
  exit 77
fi

if [[ -n "$INSTALLER_SHA256" ]]; then
  actual="$(sha256sum "$0" | awk '{print $1}')"
  if [[ "${actual,,}" != "${INSTALLER_SHA256,,}" ]]; then
    echo "installer digest mismatch" >&2
    exit 3
  fi
fi

curl_https() {
  local insecure=()
  if [[ "${PVE_ZFS_MONITOR_CURL_INSECURE:-}" == "1" ]]; then
    insecure=(-k)
  fi
  curl --proto '=https' --tlsv1.2 --proto-redir '=https' "${insecure[@]}" -fsSL "$1" -o "$2"
}

verify_bundle() {
  local manifest="$1" sig="$2" artifact="$3"
  python3 - "$manifest" "$sig" "$artifact" "$TRUST_KEY_FILE" "$CHANNEL" "$VERSION" "$GOARCH" <<'PY'
import base64, hashlib, json, os, subprocess, sys, tempfile

manifest, sig_path, artifact, key_path, channel, version, goarch = sys.argv[1:8]
with open(key_path, "r", encoding="utf-8") as f:
    pub_b64 = f.read().strip()
with open(manifest, "rb") as f:
    manifest_raw = f.read()
with open(sig_path, "r", encoding="utf-8") as f:
    env = json.load(f)
if env.get("algorithm") != "ed25519":
    sys.exit(3)
sig = base64.b64decode(env["signature"])
raw_pub = base64.b64decode(pub_b64)

def pem(raw32: bytes) -> bytes:
    der = b"\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00" + raw32
    b64 = base64.encodebytes(der).decode().strip()
    lines = ["-----BEGIN PUBLIC KEY-----"] + [b64[i:i+64] for i in range(0, len(b64), 64)] + ["-----END PUBLIC KEY-----"]
    return ("\n".join(lines) + "\n").encode()

with tempfile.TemporaryDirectory() as td:
    pemf = os.path.join(td, "pub.pem")
    msgf = os.path.join(td, "msg.bin")
    sigf = os.path.join(td, "sig.bin")
    open(pemf, "wb").write(pem(raw_pub))
    open(msgf, "wb").write(manifest_raw)
    open(sigf, "wb").write(sig)
    p = subprocess.run(["openssl", "pkeyutl", "-verify", "-pubin", "-inkey", pemf, "-rawin", "-in", msgf, "-sigfile", sigf], capture_output=True)
    if p.returncode != 0:
        sys.exit(3)

doc = json.loads(manifest_raw.decode())
if doc.get("channel") != channel or doc.get("version") != version:
    sys.exit(3)
art = next((a for a in doc.get("artifacts", []) if a.get("goos") == "linux" and a.get("goarch") == goarch), None)
if not art:
    sys.exit(3)
name = art.get("filename", "")
if not name or "/" in name or ".." in name:
    sys.exit(3)
data = open(artifact, "rb").read()
if len(data) != int(art["size"]):
    sys.exit(3)
if hashlib.sha256(data).hexdigest().lower() != art["sha256"].lower():
    sys.exit(3)
print("verified", doc["version"], name)
PY
}

atomic_install_file() {
  local verified="$1"
  local stage="${TARGET}.staging"
  if [[ -L "$TARGET" ]]; then
    echo "refusing symlink target" >&2
    return 3
  fi
  install -m 0755 "$verified" "$stage"
  if [[ -f "$TARGET" ]]; then
    install -m 0755 "$TARGET" "${ROLLBACK}.new"
    mv -f "${ROLLBACK}.new" "$ROLLBACK"
  fi
  mv -f "$stage" "$TARGET"
}

persist_release_config() {
  mkdir -p "$CONF_DIR"
  install -m 0644 "$TRUST_KEY_FILE" "$CONF_DIR/trusted-signing-key.pub.b64"
  chown root:root "$CONF_DIR/trusted-signing-key.pub.b64" 2>/dev/null || true
  local tmp="$CONF_DIR/.release.conf.tmp"
  cat >"$tmp" <<EOF
{
  "release_base": "${RELEASE_BASE%/}",
  "channel": "${CHANNEL}",
  "trust_key_path": "${CONF_DIR}/trusted-signing-key.pub.b64"
}
EOF
  chmod 0644 "$tmp"
  chown root:root "$tmp" 2>/dev/null || true
  mv -f "$tmp" "$CONF_DIR/release.conf"
}

mkdir -p "$(dirname "$LOCK_PATH")"
exec 9>"$LOCK_PATH"
if ! flock -n 9; then
  echo "another install/update is in progress" >&2
  exit 3
fi

TMP="$(mktemp -d /var/tmp/pve-zfs-monitor-install.XXXXXX)"
cleanup() { rm -rf "$TMP"; }
trap cleanup EXIT

PREFIX="${RELEASE_BASE%/}/${CHANNEL}/${VERSION}"
curl_https "${PREFIX}/manifest.json" "$TMP/manifest.json"
curl_https "${PREFIX}/manifest.json.sig.json" "$TMP/manifest.json.sig.json"
ARTIFACT_NAME="pve-zfs-monitor-linux-${GOARCH}"
curl_https "${PREFIX}/${ARTIFACT_NAME}" "$TMP/${ARTIFACT_NAME}"

verify_bundle "$TMP/manifest.json" "$TMP/manifest.json.sig.json" "$TMP/${ARTIFACT_NAME}"

persist_release_config
atomic_install_file "$TMP/${ARTIFACT_NAME}"

if command -v pve-zfs-monitor >/dev/null 2>&1; then
  : # already installed via shell path; config persisted for update --check
fi

echo "Installed pve-zfs-monitor to $TARGET"
