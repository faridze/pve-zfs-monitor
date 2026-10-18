#!/usr/bin/env bash
# Verify an Ed25519 PEM private key matches the approved lab public fingerprint.
set -euo pipefail

KEY_PEM="${1:-}"
EXPECTED="${EXPECTED_LAB_SIGNING_KEY_FINGERPRINT:-sha256:1307dc2b092cd88dde7f27461f3c5f48620889f870b25fe5515f427e54e14c8a}"

if [[ -z "$KEY_PEM" || ! -f "$KEY_PEM" ]]; then
  echo "usage: $0 /path/to/lab-signing.pem" >&2
  exit 64
fi

FP="$(python3 - "$KEY_PEM" <<'PY'
import base64, hashlib, subprocess, sys, tempfile

pem_path = sys.argv[1]
with tempfile.TemporaryDirectory() as td:
    pub_pem = f"{td}/pub.pem"
    pub_der = f"{td}/pub.der"
    subprocess.run(["openssl", "pkey", "-in", pem_path, "-pubout", "-out", pub_pem], check=True, capture_output=True)
    subprocess.run(["openssl", "pkey", "-pubin", "-in", pub_pem, "-outform", "DER", "-out", pub_der], check=True, capture_output=True)
    der = open(pub_der, "rb").read()
    raw = der[-32:]
    print("sha256:" + hashlib.sha256(raw).hexdigest())
PY
)"

if [[ "$FP" != "$EXPECTED" ]]; then
  echo "lab signing key fingerprint mismatch (expected $EXPECTED, got $FP)" >&2
  exit 3
fi
echo "lab signing key fingerprint OK"
