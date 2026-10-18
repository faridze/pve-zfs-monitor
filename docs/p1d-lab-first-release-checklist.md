# First authorized non-production lab release (maintainer gate)

No step in this checklist creates keys, publishes artifacts, or installs on production nodes without explicit human approval.

## Authorization (human)

- [ ] Approved immutable `GIT_SHA` recorded
- [ ] Lab SemVer chosen (`1.0.0-rc1`, etc.)
- [ ] Offline Ed25519 key exists; **public** key + fingerprint distributed out-of-band
- [ ] `install.sh` digest distributed out-of-band (not release-host-only)

## Build and sign (offline where required)

- [ ] `build-lab-artifacts.sh` with `RELEASE_BASE` **without** `/lab` suffix
- [ ] `sign-manifest-offline.sh` → `manifest.json.sig.json`
- [ ] `verify-bundle-offline.sh` on version directory

## HTTPS publication (manual)

- [ ] Upload full `lab/$VERSION/` tree; confirm immutable artifact URLs work
- [ ] `verify-release-origin.sh` against final `RELEASE_BASE`
- [ ] `publish-lab-latest.sh` locally; upload `lab/latest/` using hosting-safe promotion (see runbook §7)
- [ ] Expect `trusted_release: false` / `development: true` on lab binaries unless a separate trusted pipeline was explicitly approved

## Operator validation (PVE 8/9 staging)

- [ ] `docs/p1d-lab-acceptance.md` completed with redacted logs
