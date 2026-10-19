#!/usr/bin/env bash
# Acceptance test: publicly signed lab rc1 -> rc2 -> rollback, inside an
# ephemeral Debian 13 Docker container on GitHub-hosted Actions only.
# Production nodes MUST NOT run this script.
set -euo pipefail
umask 077

BASE="https://faridze.github.io/pve-zfs-monitor"
EXPECTED_KEY_SHA256="1307dc2b092cd88dde7f27461f3c5f48620889f870b25fe5515f427e54e14c8a"
REVIEWED_INSTALLER="/src/scripts/install.sh"
TARGET="/usr/local/bin/pve-zfs-monitor"
CFG="/etc/pve-zfs-monitor/release.conf"
KEY="/etc/pve-zfs-monitor/trusted-signing-key.pub.b64"

die() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
pass() { printf 'PASS: %s\n' "$*"; }

# Prevent accidental execution on the GitHub runner host or a PVE machine.
test "$(id -u)" -eq 0 || die "container root required"
test -f /.dockerenv || die "Docker container marker missing"
test -r "$REVIEWED_INSTALLER" || die "reviewed installer not mounted"
test ! -e "$TARGET" || die "test container already has installed binary"
test ! -e "$CFG" || die "test container already has release config"
test -z "${PVE_ZFS_MONITOR_HERMETIC_TEST:-}" || die "must exercise production paths"
test -z "${PVE_ZFS_MONITOR_TRUST_KEY_FILE:-}" || die "no inherited signing configuration"
test -z "${PVE_ZFS_MONITOR_RELEASE_BASE:-}" || die "no inherited release configuration"

TMP="$(mktemp -d -t pve-zfs-public-lab-smoke.XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

download() {
    curl --proto '=https' --proto-redir '=https' --tlsv1.2 \
        -fLsS --connect-timeout 15 --max-time 90 --retry 2 \
        --output "$2" "$BASE/$1"
}

# Independent sources for bootstrap: reviewed checked-out installer code,
# pinned public-key fingerprint known before HTTPS downloads.
download "scripts/install.sh" "$TMP/install.sh"
download "scripts/trusted-signing-key.pub.b64" "$TMP/pub.b64"

cmp -s "$REVIEWED_INSTALLER" "$TMP/install.sh" ||
    die "published installer differs from reviewed source"
INSTALLER_SHA256="$(sha256sum "$REVIEWED_INSTALLER" | cut -d' ' -f1)"
echo "$INSTALLER_SHA256  $TMP/install.sh" | sha256sum -c --status ||
    die "installer digest mismatch"
pass "published installer byte-for-byte identical to reviewed source"

python3 - "$TMP/pub.b64" "$EXPECTED_KEY_SHA256" <<'PY'
import base64
import hashlib
import pathlib
import sys
raw = base64.b64decode(pathlib.Path(sys.argv[1]).read_text().strip(), validate=True)
if len(raw) != 32 or hashlib.sha256(raw).hexdigest() != sys.argv[2]:
    sys.exit("FAIL: downloaded Ed25519 public-key fingerprint mismatch")
print("PASS: pinned Ed25519 public-key fingerprint")
PY

expect_version() {
    local want="$1"
    "$TARGET" version --json | python3 -c '
import json, sys
want = sys.argv[1]
doc = json.load(sys.stdin)
if (doc.get("version"), doc.get("channel"), doc.get("goos"), doc.get("goarch")) != (want, "lab", "linux", "amd64"):
    sys.exit("FAIL: installed version/platform/channel mismatch: " + repr(doc))
print("PASS: installed " + want + " lab linux/amd64")
' "$want"
}

file_sha() { sha256sum "$1" | cut -d' ' -f1; }

# This is the actual unmodified production-path installer, not hermetic mode.
INSTALLER_SHA256="$INSTALLER_SHA256" \
    timeout -k 5s 180s bash "$TMP/install.sh" \
        --channel lab --version 1.0.0-rc1 \
        --release-base "$BASE" --trust-key-file "$TMP/pub.b64"
expect_version "1.0.0-rc1"
test -f "$CFG" && test -f "$KEY" || die "bootstrap config or trust key missing"
test ! -e "$TARGET.previous" || die "initial install unexpectedly created rollback"
rc1sha="$(file_sha "$TARGET")"
config_sha="$(file_sha "$CFG")"
key_sha="$(file_sha "$KEY")"

# Fresh process, relying on persisted configuration and signing trust.
check="$(timeout -k 5s 90s "$TARGET" update --check)"
printf '%s\n' "$check" | grep -Fq "update available: 1.0.0-rc1 -> 1.0.0-rc2" ||
    die "signed update not advertised from rc1 to rc2"
pass "signed latest metadata advertises rc2"

dry="$(timeout -k 5s 180s "$TARGET" update --dry-run)"
printf '%s\n' "$dry" | grep -Fq "dry-run: manifest signature, artifact size and sha256 verified" ||
    die "update dry-run did not verify signed artifact"
test "$(file_sha "$TARGET")" = "$rc1sha" ||
    die "dry-run mutated installed binary"
test ! -e "$TARGET.previous" ||
    die "dry-run created rollback file"
pass "dry-run verified real rc2 without replacing rc1"

timeout -k 5s 180s "$TARGET" update
expect_version "1.0.0-rc2"
test "$(file_sha "$TARGET")" != "$rc1sha" ||
    die "update did not change installed binary"
test -f "$TARGET.previous" && test "$(file_sha "$TARGET.previous")" = "$rc1sha" ||
    die "rc1 rollback copy missing or incorrect"
pass "real signed rc1 to rc2 update and previous binary"

check="$(timeout -k 5s 90s "$TARGET" update --check)"
printf '%s\n' "$check" | grep -Fq "already up to date: 1.0.0-rc2" ||
    die "post-update latest check did not report current"
pass "rc2 already up to date"

timeout -k 5s 30s "$TARGET" rollback
expect_version "1.0.0-rc1"
test "$(file_sha "$TARGET")" = "$rc1sha" ||
    die "rollback did not restore original rc1 bytes"
test "$(file_sha "$CFG")" = "$config_sha" &&
test "$(file_sha "$KEY")" = "$key_sha" ||
    die "rollback unexpectedly changed release config/trust"
pass "rollback restored exactly original rc1 and preserved config/trust"

printf 'VERIFIED: real public lab rc1 -> rc2 -> rc1 on isolated Debian 13 container\n'
