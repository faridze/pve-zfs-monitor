#!/usr/bin/env bash
# Self-contained pve-zfs-monitor installer (no Git/Go). All verifier/install logic is embedded in this file.
#
# Operator trust (required before root execution):
#   1. Obtain install.sh.sha256 (or signature) from maintainer out-of-band — NOT from the release host alone.
#   2. sha256sum -c install.sh.sha256
#   3. sudo INSTALLER_SHA256=<same-hex> bash install.sh ...
set -euo pipefail

HERMETIC="${PVE_ZFS_MONITOR_HERMETIC_TEST:-}"
DEFAULT_LOCK_DIR="/run/lock/pve-zfs-monitor"
DEFAULT_LOCK_PATH="${DEFAULT_LOCK_DIR}/install.lock"
DEFAULT_TARGET="/usr/local/bin/pve-zfs-monitor"
DEFAULT_CONF_DIR="/etc/pve-zfs-monitor"

LOCK_PATH="${DEFAULT_LOCK_PATH}"
TARGET="${DEFAULT_TARGET}"
ROLLBACK="${TARGET}.previous"
CONF_DIR="${DEFAULT_CONF_DIR}"

CHANNEL=""
VERSION=""
RELEASE_BASE="${PVE_ZFS_MONITOR_RELEASE_BASE:-}"
TRUST_KEY_FILE="${PVE_ZFS_MONITOR_TRUST_KEY_FILE:-}"
INSTALLER_SHA256="${INSTALLER_SHA256:-}"
ALLOW_TRUST_KEY_ROTATION="${PVE_ZFS_MONITOR_ALLOW_TRUST_KEY_ROTATION:-0}"

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) GOARCH=amd64 ;;
  aarch64|arm64) GOARCH=arm64 ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

usage() {
  echo "Usage: sha256sum -c install.sh.sha256  # maintainer file, out-of-band" >&2
  echo "       sudo INSTALLER_SHA256=<hex> bash $0 --channel stable|lab --version VER --release-base URL --trust-key-file PATH" >&2
  exit 64
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --channel) CHANNEL="$2"; shift 2 ;;
    --version) VERSION="$2"; shift 2 ;;
    --release-base) RELEASE_BASE="$2"; shift 2 ;;
    --trust-key-file) TRUST_KEY_FILE="$2"; shift 2 ;;
    --allow-trust-key-rotation) ALLOW_TRUST_KEY_ROTATION=1; shift ;;
    -h|--help) usage ;;
    *) echo "unknown argument: $1" >&2; usage ;;
  esac
done

[[ -n "$CHANNEL" && -n "$VERSION" && -n "$RELEASE_BASE" && -n "$TRUST_KEY_FILE" ]] || usage
[[ "$CHANNEL" == "stable" || "$CHANNEL" == "lab" ]] || usage
case "$RELEASE_BASE" in https://*) ;; *) echo "release base must be https://" >&2; exit 3 ;; esac

if [[ -z "$INSTALLER_SHA256" ]]; then
  echo "INSTALLER_SHA256 required (verify install.sh with maintainer digest before sudo)" >&2
  exit 3
fi
actual="$(sha256sum "$0" | awk '{print $1}')"
if [[ "${actual,,}" != "${INSTALLER_SHA256,,}" ]]; then
  echo "installer digest mismatch" >&2
  exit 3
fi

[[ -f "$TRUST_KEY_FILE" ]] || { echo "trust key file not found" >&2; exit 3; }

if [[ "$HERMETIC" == "1" ]]; then
  LOCK_PATH="${PVE_ZFS_MONITOR_LOCK_PATH:-$DEFAULT_LOCK_PATH}"
  TARGET="${PVE_ZFS_MONITOR_INSTALL_TARGET:-$DEFAULT_TARGET}"
  ROLLBACK="${TARGET}.previous"
  CONF_DIR="${PVE_ZFS_MONITOR_CONFIG_DIR:-$DEFAULT_CONF_DIR}"
else
  if [[ -n "${PVE_ZFS_MONITOR_CURL_INSECURE:-}" ]]; then
    echo "refusing disabled TLS verification in production" >&2
    exit 3
  fi
  for v in "${PVE_ZFS_MONITOR_LOCK_PATH:-}" "${PVE_ZFS_MONITOR_INSTALL_TARGET:-}" "${PVE_ZFS_MONITOR_CONFIG_DIR:-}"; do
    if [[ -n "$v" ]]; then
      echo "refusing non-default install paths in production" >&2
      exit 3
    fi
  done
fi

command -v python3 >/dev/null || { echo "python3 required" >&2; exit 3; }
command -v openssl >/dev/null || { echo "openssl required" >&2; exit 3; }
command -v curl >/dev/null || { echo "curl required" >&2; exit 3; }
command -v flock >/dev/null || { echo "flock required" >&2; exit 3; }

if [[ "$HERMETIC" != "1" ]] && [[ "$(id -u)" -ne 0 ]]; then
  echo "root privileges required (use sudo)" >&2
  exit 77
fi

curl_https() {
  local insecure=()
  if [[ "$HERMETIC" == "1" && "${PVE_ZFS_MONITOR_CURL_INSECURE:-}" == "1" ]]; then
    insecure=(-k)
  fi
  curl --proto '=https' --tlsv1.2 --proto-redir '=https' "${insecure[@]}" -fsSL "$1" -o "$2"
}

reject_symlink() {
  local p="$1"
  if [[ -L "$p" ]]; then
    echo "refusing symlink at $p" >&2
    return 3
  fi
  return 0
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

CONFIG_BACKUP_DIR=""
backup_release_config() {
  CONFIG_BACKUP_DIR="$(mktemp -d -t pve-zfs-monitor-config-backup.XXXXXX)"
  local conf="${CONF_DIR}/release.conf"
  local key="${CONF_DIR}/trusted-signing-key.pub.b64"
  if [[ -f "$conf" ]]; then cp -a "$conf" "${CONFIG_BACKUP_DIR}/release.conf"; fi
  if [[ -f "$key" ]]; then cp -a "$key" "${CONFIG_BACKUP_DIR}/trusted-signing-key.pub.b64"; fi
}

restore_release_config() {
  [[ -z "$CONFIG_BACKUP_DIR" ]] && return 0
  local conf="${CONF_DIR}/release.conf"
  local key="${CONF_DIR}/trusted-signing-key.pub.b64"
  if [[ -f "${CONFIG_BACKUP_DIR}/release.conf" ]]; then
    install -m 0644 "${CONFIG_BACKUP_DIR}/release.conf" "$conf"
    chown root:root "$conf" 2>/dev/null || true
  else
    rm -f "$conf"
  fi
  if [[ -f "${CONFIG_BACKUP_DIR}/trusted-signing-key.pub.b64" ]]; then
    install -m 0644 "${CONFIG_BACKUP_DIR}/trusted-signing-key.pub.b64" "$key"
    chown root:root "$key" 2>/dev/null || true
  else
    rm -f "$key"
  fi
  rm -rf "$CONFIG_BACKUP_DIR"
  CONFIG_BACKUP_DIR=""
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

validate_persisted_trust_anchor() {
  local conf="${CONF_DIR}/release.conf"
  local persisted="${CONF_DIR}/trusted-signing-key.pub.b64"
  [[ -f "$conf" ]] || return 0
  python3 - "$persisted" "$TRUST_KEY_FILE" "$ALLOW_TRUST_KEY_ROTATION" <<'PY'
import base64, sys
persisted_path, supplied_path, allow = sys.argv[1:4]
if not __import__("os").path.isfile(persisted_path):
    sys.exit(0)
with open(persisted_path, encoding="utf-8") as f:
    a = f.read().strip()
with open(supplied_path, encoding="utf-8") as f:
    b = f.read().strip()
if base64.b64decode(a) == base64.b64decode(b):
    sys.exit(0)
if allow == "1":
    sys.exit(0)
print("trust signing key does not match persisted anchor (see docs/p1d-lab-key-rotation.md)", file=sys.stderr)
sys.exit(3)
PY
}

should_skip_config_rewrite() {
  local conf="${CONF_DIR}/release.conf"
  [[ -f "$conf" ]] || return 1
  python3 - "$conf" "$CHANNEL" "$RELEASE_BASE" "$TRUST_KEY_FILE" "${CONF_DIR}/trusted-signing-key.pub.b64" <<'PY'
import base64, json, sys
path, channel, base, supplied_path, persisted_path = sys.argv[1:6]
with open(path, encoding="utf-8") as f:
    doc = json.load(f)
base = base.rstrip("/")
rb = doc.get("release_base", "").rstrip("/")
if doc.get("channel") != channel or rb != base:
    sys.exit(1)
if not __import__("os").path.isfile(persisted_path):
    sys.exit(1)
with open(persisted_path, encoding="utf-8") as f:
    a = f.read().strip()
with open(supplied_path, encoding="utf-8") as f:
    b = f.read().strip()
sys.exit(0 if base64.b64decode(a) == base64.b64decode(b) else 1)
PY
}

acquire_install_lock() {
  reject_symlink "$(dirname "$LOCK_PATH")" || exit 3
  reject_symlink "$LOCK_PATH" || exit 3
  mkdir -p "$(dirname "$LOCK_PATH")"
  chmod 0755 "$(dirname "$LOCK_PATH")" 2>/dev/null || true
  chown root:root "$(dirname "$LOCK_PATH")" 2>/dev/null || true
  if [[ ! -e "$LOCK_PATH" ]]; then
    install -m 0600 /dev/null "$LOCK_PATH"
  fi
  exec 9>>"$LOCK_PATH"
  if ! flock -n 9; then
    echo "another install/update is in progress" >&2
    exit 3
  fi
}

acquire_install_lock

TMP="$(mktemp -d -t pve-zfs-monitor-install.XXXXXX)"
cleanup() {
  rm -rf "$TMP"
  if [[ -n "$CONFIG_BACKUP_DIR" ]]; then
    restore_release_config
  fi
}
trap cleanup EXIT

PREFIX="${RELEASE_BASE%/}/${CHANNEL}/${VERSION}"
curl_https "${PREFIX}/manifest.json" "$TMP/manifest.json"
curl_https "${PREFIX}/manifest.json.sig.json" "$TMP/manifest.json.sig.json"
ARTIFACT_NAME="pve-zfs-monitor-linux-${GOARCH}"
curl_https "${PREFIX}/${ARTIFACT_NAME}" "$TMP/${ARTIFACT_NAME}"

verify_bundle "$TMP/manifest.json" "$TMP/manifest.json.sig.json" "$TMP/${ARTIFACT_NAME}"

validate_persisted_trust_anchor

if should_skip_config_rewrite; then
  if ! atomic_install_file "$TMP/${ARTIFACT_NAME}"; then
    exit 3
  fi
else
  backup_release_config
  persist_release_config
  if ! atomic_install_file "$TMP/${ARTIFACT_NAME}"; then
    restore_release_config
    exit 3
  fi
  rm -rf "$CONFIG_BACKUP_DIR"
  CONFIG_BACKUP_DIR=""
fi

echo "Installed pve-zfs-monitor to $TARGET"
