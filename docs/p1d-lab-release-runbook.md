# P1D lab release runbook (maintainer, offline signing)

**No private keys in git, PR CI, or public artifacts.** This runbook prepares bundles only; publishing requires separate maintainer approval.

## Prerequisites

- Approved immutable `GIT_SHA` on `main` or a reviewed release branch.
- Offline Ed25519 private key (maintainer hardware / HSM).
- HTTPS release storage with separate `lab/` and `stable/` prefixes (never mix channels).

## 1. Build unsigned artifacts (online build host)

```bash
export GIT_SHA='<approved-full-sha>'
export VERSION='1.0.0-rc1'          # lab SemVer only
export RELEASE_BASE='https://releases.example.invalid'
export OUT_DIR='dist/lab'

./release/signing/build-lab-artifacts.sh
```

Produces `dist/lab/$VERSION/pve-zfs-monitor-linux-{amd64,arm64}` and unsigned `manifest.json`.

## 2. Sign manifest offline

On an **air-gapped or offline** machine with the private key PEM:

```bash
export PVE_ZFS_MONITOR_SIGNING_KEY_PEM=/secure/offline/lab-signing.pem
./release/signing/sign-manifest-offline.sh dist/lab/$VERSION/manifest.json
```

## 3. Distribute trust material independently

- `trusted-signing-key.pub.b64` (public key only)
- `sha256:` fingerprint via `./release/signing/fingerprint-pubkey.sh trusted-signing-key.pub.b64`
- `install.sh.sha256` for the pinned installer script digest (not from the release host alone)

## 4. Verify bundle before upload

```bash
./release/signing/verify-bundle-offline.sh dist/lab/$VERSION trusted-signing-key.pub.b64
```

CI uses **synthetic** keys in `internal/release/lab_bundle_test.go` only.

## 5. Upload (manual, post-approval)

Upload to `https://.../lab/$VERSION/`:

- `manifest.json`
- `manifest.json.sig.json`
- `pve-zfs-monitor-linux-amd64`
- `pve-zfs-monitor-linux-arm64`

Do **not** tag, merge, or deploy from this step.
