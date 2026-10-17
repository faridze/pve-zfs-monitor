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

## 5. Upload versioned tree (manual, post-approval)

`RELEASE_BASE` is the HTTPS origin **without** a channel suffix (installer/updater append `/lab/...`).

Upload **all** versioned objects to `https://<host>/lab/$VERSION/`:

- `manifest.json`
- `manifest.json.sig.json`
- `pve-zfs-monitor-linux-amd64`
- `pve-zfs-monitor-linux-arm64`

## 6. Verify through the final HTTPS origin

After upload, confirm URLs in the signed manifest resolve and match hashes at the **public origin** (not only on the build host):

```bash
export RELEASE_BASE='https://releases.example.invalid'
export VERIFY_HTTPS_ORIGIN=1
./release/signing/verify-release-origin.sh dist/lab/$VERSION trusted-signing-key.pub.b64 "$RELEASE_BASE"
```

Or call `verify-release-origin.sh` directly before promoting `latest`.

## 7. Promote `lab/latest` metadata (hosting-aware)

Generic HTTPS object storage does **not** atomically replace manifest + signature as a pair. Remote readers may briefly observe a mismatched pair if objects are uploaded sequentially; `pve-zfs-monitor update` **fails closed** (no binary change) when verification fails.

Safe promotion process:

1. Finish and verify the immutable version directory at the HTTPS origin.
2. Prepare local `latest` metadata from that version (does not remove an existing `latest/` directory):

```bash
./release/signing/publish-lab-latest.sh dist/lab/$VERSION dist/lab trusted-signing-key.pub.b64
```

3. Upload `dist/lab/latest/manifest.json` and `dist/lab/latest/manifest.json.sig.json` using a **hosting-appropriate** method:
   - Prefer uploading to a staging prefix, then a single pointer swap if your platform supports it (CDN cache purge, reverse-proxy directory switch, S3 website redirect, etc.).
   - If only per-object PUT is available, upload the **signature first** or accept a short window where `update --check` may fail until both objects match.
   - Never upload `latest` before the versioned artifacts are live.

`publish-lab-latest.sh` performs **local** in-place file replacement under `lab/latest/` and preserves the previous `latest` tree if promotion fails or is interrupted.

`pve-zfs-monitor update` fetches `{RELEASE_BASE}/lab/latest/manifest.json` (not stable).

## Release metadata (`trusted_release`)

- `build-lab-artifacts.sh` produces binaries with `TRUSTED=false` (reports `development: true`, `trusted_release: false`).
- Manifest Ed25519 signatures prove release integrity; they do **not** flip `trusted_release`.
- Only a separately approved maintainer release pipeline may set `TRUSTED=true` at link time with pinned public key material (`docs/install.md`).

Do **not** tag, merge, or deploy from this step.
