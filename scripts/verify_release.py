#!/usr/bin/env python3
"""Verify signed release manifest and artifact without executing the artifact (PVE 8/9 bootstrap)."""
from __future__ import annotations

import argparse
import base64
import hashlib
import json
import os
import subprocess
import sys
import tempfile


def require_tool(name: str) -> None:
    from shutil import which

    if which(name) is None:
        sys.stderr.write(f"required tool not found: {name}\n")
        sys.exit(3)


def ed25519_public_pem(raw32: bytes) -> bytes:
    if len(raw32) != 32:
        raise ValueError("ed25519 public key must be 32 bytes")
    # SPKI DER prefix for Ed25519
    der = (
        b"\x30\x2a"
        b"\x30\x05\x06\x03\x2b\x65\x70"
        b"\x03\x21\x00" + raw32
    )
    b64 = base64.encodebytes(der).decode("ascii").strip()
    lines = [f"-----BEGIN PUBLIC KEY-----"] + [
        b64[i : i + 64] for i in range(0, len(b64), 64)
    ] + ["-----END PUBLIC KEY-----"]
    return ("\n".join(lines) + "\n").encode("ascii")


def verify_ed25519_openssl(pub_b64: str, message: bytes, sig_b64: str) -> None:
    raw_pub = base64.b64decode(pub_b64.strip())
    sig = base64.b64decode(sig_b64.strip())
    with tempfile.TemporaryDirectory() as td:
        pem = os.path.join(td, "pub.pem")
        msg = os.path.join(td, "message.bin")
        sigf = os.path.join(td, "sig.bin")
        with open(pem, "wb") as f:
            f.write(ed25519_public_pem(raw_pub))
        with open(msg, "wb") as f:
            f.write(message)
        with open(sigf, "wb") as f:
            f.write(sig)
        proc = subprocess.run(
            [
                "openssl",
                "pkeyutl",
                "-verify",
                "-pubin",
                "-inkey",
                pem,
                "-rawin",
                "-in",
                msg,
                "-sigfile",
                sigf,
            ],
            capture_output=True,
        )
        if proc.returncode != 0:
            sys.stderr.write(proc.stderr.decode() or "signature verification failed\n")
            sys.exit(3)


def main() -> None:
    require_tool("openssl")
    require_tool("python3")
    p = argparse.ArgumentParser()
    p.add_argument("--manifest", required=True)
    p.add_argument("--signature", required=True)
    p.add_argument("--artifact", required=True)
    p.add_argument("--trust-key-file", required=True)
    p.add_argument("--channel", required=True)
    p.add_argument("--version", required=True)
    p.add_argument("--goarch", required=True)
    args = p.parse_args()

    with open(args.trust_key_file, "r", encoding="utf-8") as f:
        pub_b64 = f.read().strip()
    with open(args.manifest, "rb") as f:
        manifest_raw = f.read()
    with open(args.signature, "r", encoding="utf-8") as f:
        sig_env = json.load(f)
    if sig_env.get("algorithm") != "ed25519":
        sys.stderr.write("unsupported signature algorithm\n")
        sys.exit(3)
    verify_ed25519_openssl(pub_b64, manifest_raw, sig_env["signature"])

    manifest = json.loads(manifest_raw.decode("utf-8"))
    if manifest.get("channel") != args.channel:
        sys.stderr.write("channel mismatch\n")
        sys.exit(3)
    if manifest.get("version") != args.version:
        sys.stderr.write("version mismatch\n")
        sys.exit(3)
    arts = manifest.get("artifacts") or []
    art = None
    for candidate in arts:
        if candidate.get("goos") == "linux" and candidate.get("goarch") == args.goarch:
            art = candidate
            break
    if art is None:
        sys.stderr.write("no matching artifact in manifest\n")
        sys.exit(3)
    name = art.get("filename", "")
    if not name or "/" in name or ".." in name:
        sys.stderr.write("invalid artifact filename\n")
        sys.exit(3)
    size = int(art["size"])
    want = art["sha256"].lower()
    with open(args.artifact, "rb") as f:
        data = f.read()
    if len(data) != size:
        sys.stderr.write("artifact size mismatch\n")
        sys.exit(3)
    got = hashlib.sha256(data).hexdigest()
    if got.lower() != want:
        sys.stderr.write("artifact sha256 mismatch\n")
        sys.exit(3)
    print("verified", manifest["version"], art["filename"])


if __name__ == "__main__":
    main()
