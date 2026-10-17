#!/usr/bin/env bash
# Offline maintainer signing — NEVER run in PR CI. Private key path is local only.
set -euo pipefail

MANIFEST="${1:-}"
PRIVATE_KEY_PEM="${PVE_ZFS_MONITOR_SIGNING_KEY_PEM:-}"

if [[ -z "$MANIFEST" || -z "$PRIVATE_KEY_PEM" ]]; then
  echo "Usage: PVE_ZFS_MONITOR_SIGNING_KEY_PEM=/secure/offline/key.pem $0 /path/to/manifest.json" >&2
  exit 64
fi
if [[ ! -f "$PRIVATE_KEY_PEM" ]]; then
  echo "private key not found" >&2
  exit 3
fi

SIG_OUT="${MANIFEST}.sig.json"
python3 - "$MANIFEST" "$PRIVATE_KEY_PEM" "$SIG_OUT" <<'PY'
import base64, json, subprocess, sys, tempfile

manifest, key_pem, out = sys.argv[1:4]
with open(manifest, "rb") as f:
    raw = f.read()
with tempfile.TemporaryDirectory() as td:
    msg = f"{td}/msg.bin"
    sig = f"{td}/sig.bin"
    open(msg, "wb").write(raw)
    p = subprocess.run(
        ["openssl", "pkeyutl", "-sign", "-inkey", key_pem, "-rawin", "-in", msg, "-out", sig],
        capture_output=True,
    )
    if p.returncode != 0:
        sys.stderr.write(p.stderr.decode())
        sys.exit(3)
    sig_b64 = base64.b64encode(open(sig, "rb").read()).decode()
env = {"algorithm": "ed25519", "public_key_id": "maintainer", "signature": sig_b64}
with open(out, "w", encoding="utf-8") as f:
    json.dump(env, f, indent=2)
    f.write("\n")
print("wrote", out)
PY
