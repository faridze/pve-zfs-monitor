# Release signing

## Status

**No production signing key is configured in this repository.**

P1D implements verification hooks only. Publishing trusted `stable` or `lab` binaries requires explicit maintainer approval and an isolated signing workflow.

## Manifest schema (v1)

- `manifest.json` — version, channel, commit, artifacts (filename, goos, goarch, sha256, size, url)
- `manifest.json.sig.json` — `{ "algorithm": "ed25519", "public_key_id": "...", "signature": "<base64>" }`

## Verification

Consumers pin trust via:

- Link-time `TrustedPublicKeyB64` on official builds, or
- Environment variable `PVE_ZFS_MONITOR_TRUST_KEY_FILE` pointing at a base64-encoded ed25519 public key.

Checksum-only verification without a signature is **not** sufficient and is not supported for install/update.

## Maintainer workflow (after approval)

1. Build static binaries per architecture.
2. Compute SHA-256 and populate `manifest.json`.
3. Sign manifest bytes with offline private key.
4. Publish manifest, signature, and binaries to HTTPS release storage.
5. Document public key fingerprint in release notes.

Never commit private keys, tokens, or unsigned binaries labeled as trusted.
