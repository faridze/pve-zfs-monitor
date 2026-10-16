#!/usr/bin/env bash
# Verify a release directory using a trusted public key (maintainer or synthetic test key).
set -euo pipefail

DIR="${1:-}"
PUB_B64="${2:-}"

if [[ -z "$DIR" || -z "$PUB_B64" || ! -f "$PUB_B64" ]]; then
  echo "Usage: $0 /path/to/lab/VERSION /path/to/trusted-signing-key.pub.b64" >&2
  exit 64
fi

MANIFEST="$DIR/manifest.json"
SIG="$DIR/manifest.json.sig.json"
if [[ ! -f "$MANIFEST" || ! -f "$SIG" ]]; then
  echo "missing manifest or signature" >&2
  exit 3
fi

python3 - "$MANIFEST" "$SIG" "$PUB_B64" "$DIR" <<'PY'
import base64, hashlib, json, os, subprocess, sys, tempfile

manifest, sig_path, pub_path, bundle_dir = sys.argv[1:5]
with open(manifest, "rb") as f:
    raw = f.read()
with open(sig_path, encoding="utf-8") as f:
    env = json.load(f)
sig = base64.b64decode(env["signature"])
raw_pub = base64.b64decode(open(pub_path).read().strip())

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
    open(msgf, "wb").write(raw)
    open(sigf, "wb").write(sig)
    p = subprocess.run(["openssl", "pkeyutl", "-verify", "-pubin", "-inkey", pemf, "-rawin", "-in", msgf, "-sigfile", sigf], capture_output=True)
    if p.returncode != 0:
        sys.exit(3)

doc = json.loads(raw.decode())
for art in doc.get("artifacts", []):
    path = os.path.join(bundle_dir, art["filename"])
    data = open(path, "rb").read()
    if len(data) != int(art["size"]):
        sys.exit(3)
    if hashlib.sha256(data).hexdigest().lower() != art["sha256"].lower():
        sys.exit(3)
print("bundle OK", doc["channel"], doc["version"])
PY
