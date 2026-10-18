#!/usr/bin/env bash
set -euo pipefail
RELEASE_BASE="${1:?}"
CHANNEL="${2:?}"
TRUST_KEY_SRC="${3:?}"
CONF_DIR="/etc/pve-zfs-monitor"
DEST_KEY="$CONF_DIR/trusted-signing-key.pub.b64"
CONF_FILE="$CONF_DIR/release.conf"

mkdir -p "$CONF_DIR"
install -m 0644 "$TRUST_KEY_SRC" "$DEST_KEY"
cat >"$CONF_FILE" <<EOF
{
  "release_base": "${RELEASE_BASE%/}",
  "channel": "${CHANNEL}",
  "trust_key_path": "${DEST_KEY}"
}
EOF
chmod 0644 "$CONF_FILE"
echo "wrote $CONF_FILE"
