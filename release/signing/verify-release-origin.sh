#!/usr/bin/env bash
# Verify manifest, signature, and artifact URLs through the final HTTPS release origin.
set -euo pipefail

MANIFEST="${1:-}"
TRUST_PUB_B64="${2:-}"
RELEASE_BASE="${3:-}"

if [[ -z "$MANIFEST" || -z "$TRUST_PUB_B64" || -z "$RELEASE_BASE" ]]; then
  echo "Usage: $0 /path/to/manifest.json trusted-signing-key.pub.b64 https://releases.example" >&2
  exit 64
fi

case "$RELEASE_BASE" in https://*) ;; *)
  echo "RELEASE_BASE must be https://" >&2
  exit 3
esac

python3 - "$MANIFEST" "$TRUST_PUB_B64" "$RELEASE_BASE" <<'PY'
import base64, hashlib, json, subprocess, sys, tempfile, urllib.request

manifest_path, pub_path, base = sys.argv[1:4]
base = base.rstrip("/")
with open(manifest_path, "rb") as f:
    raw = f.read()
doc = json.loads(raw.decode())
version = doc["version"]
channel = doc["channel"]
prefix = f"{base}/{channel}/{version}"

def get(url: str) -> bytes:
    req = urllib.request.Request(url, method="GET")
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read()

# Manifest at origin (optional cross-check when promoting from local tree)
remote_man = get(f"{prefix}/manifest.json")
if remote_man != raw:
    print("remote manifest.json does not match local approved manifest", file=sys.stderr)
    sys.exit(3)

sig_url = f"{prefix}/manifest.json.sig.json"
sig_raw = get(sig_url)
with open(pub_path, encoding="utf-8") as f:
    pub_b64 = f.read().strip()
raw_pub = base64.b64decode(pub_b64)

def pem(raw32: bytes) -> bytes:
    der = b"\x30\x2a\x30\x05\x06\x03\x2b\x65\x70\x03\x21\x00" + raw32
    b64 = base64.encodebytes(der).decode().strip()
    lines = ["-----BEGIN PUBLIC KEY-----"] + [b64[i:i+64] for i in range(0, len(b64), 64)] + ["-----END PUBLIC KEY-----"]
    return ("\n".join(lines) + "\n").encode()

with tempfile.TemporaryDirectory() as td:
    import os
    pemf = os.path.join(td, "pub.pem")
    msgf = os.path.join(td, "msg.bin")
    sigf = os.path.join(td, "sig.bin")
    open(pemf, "wb").write(pem(raw_pub))
    open(msgf, "wb").write(remote_man)
    env = json.loads(sig_raw.decode())
    open(sigf, "wb").write(base64.b64decode(env["signature"]))
    p = subprocess.run(["openssl", "pkeyutl", "-verify", "-pubin", "-inkey", pemf, "-rawin", "-in", msgf, "-sigfile", sigf], capture_output=True)
    if p.returncode != 0:
        sys.exit(3)

for art in doc.get("artifacts", []):
    url = art.get("url") or f"{prefix}/{art['filename']}"
    if not url.startswith("https://"):
        print(f"artifact url must be https: {url}", file=sys.stderr)
        sys.exit(3)
    data = get(url)
    if len(data) != int(art["size"]):
        print("artifact size mismatch at origin", file=sys.stderr)
        sys.exit(3)
    if hashlib.sha256(data).hexdigest().lower() != art["sha256"].lower():
        print("artifact sha256 mismatch at origin", file=sys.stderr)
        sys.exit(3)
print("origin OK", channel, version)
PY
