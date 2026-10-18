# Release signing

## Status

**No production signing key is configured in this repository.**

P1D implements verification hooks and **maintainer runbooks** only. Publishing trusted `lab` or `stable` binaries requires explicit maintainer approval and an isolated signing workflow.

## Layout

| Path | Purpose |
|------|---------|
| `build-lab-artifacts.sh` | Build amd64/arm64 from immutable `GIT_SHA` + unsigned manifest |
| `assemble-manifest.py` | SHA-256, size, HTTPS URLs per channel/version |
| `sign-manifest-offline.sh` | Offline Ed25519 manifest signature (`PVE_ZFS_MONITOR_SIGNING_KEY_PEM`) |
| `verify-bundle-offline.sh` | Maintainer pre-upload verification |
| `publish-lab-latest.sh` | Local in-place promotion to `lab/latest/` (preserves prior on failure) |
| `verify-release-origin.sh` | HTTPS origin verification of manifest, sig, and artifact URLs |
| `assemble-gh-pages-tree.sh` | Stage `scripts/` + `lab/VERSION/` + `lab/latest/` for GitHub Pages |
| `ci-lab-release-build.sh` | CI wrapper for unsigned lab builds (no secrets) |
| `verify-lab-signing-key.sh` | Ensure PEM private key matches approved public fingerprint |
| `fetch-existing-pages-site.sh` | Download live Pages tree before merge/deploy |
| `merge-pages-for-deploy.sh` | Merge signed version; refuse clobbering immutable releases |
| `record-published-lab-version.sh` | Maintain `lab/.published-versions` for fetch bootstrap |
| `fingerprint-pubkey.sh` | Independent public-key fingerprint for operators |

## `latest` metadata contract

Updaters fetch `{RELEASE_BASE}/lab/latest/manifest.json` (+ `.sig.json`). Versioned binaries stay at `{RELEASE_BASE}/lab/{version}/`. Publish `latest` only after the full versioned bundle is live at the HTTPS origin; never mix `stable` and `lab`.

Remote promotion is **not** claimed to be atomic on generic HTTPS storage. Use platform-specific pointer/swap patterns where available; otherwise expect transient check failures until both metadata objects match.

Offline signing writes **`manifest.json.sig.json`** (not `manifest.sig.json`).

Runbook: `docs/p1d-lab-release-runbook.md`  
GitHub Actions setup: `docs/github-actions-lab-release.md`  
Operator checklist: `docs/p1d-lab-acceptance.md`  
Key rotation: `docs/p1d-lab-key-rotation.md`

## Manifest schema (v1)

- `manifest.json` — version, channel, commit, artifacts (filename, goos, goarch, sha256, size, url)
- `manifest.json.sig.json` — `{ "algorithm": "ed25519", "public_key_id": "...", "signature": "<base64>" }`

## Channel separation

- `stable/` and `lab/` are distinct HTTPS path prefixes and manifest `channel` values.
- Installers refuse channel switches during `update`.

## Verification in CI

`internal/release/lab_bundle_test.go` exercises assembly + verification with **synthetic test keys only**.

Never commit private keys, tokens, or unsigned binaries labeled as trusted.
