#!/usr/bin/env bash
# Print SHA-256 fingerprint of a base64 ed25519 public key file for operator distribution.
set -euo pipefail
KEY="${1:-}"
if [[ -z "$KEY" || ! -f "$KEY" ]]; then
  echo "Usage: $0 trusted-signing-key.pub.b64" >&2
  exit 64
fi
python3 - "$KEY" <<'PY'
import base64, hashlib, sys
raw = base64.b64decode(open(sys.argv[1]).read().strip())
print("sha256:" + hashlib.sha256(raw).hexdigest())
PY
