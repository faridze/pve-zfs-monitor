# Installation and updates (P1D)

Read-only monitoring binary distribution for Proxmox VE nodes **without Git or Go**.

## Trust model

Releases are described by a **signed manifest** (ed25519). The installer and `pve-zfs-monitor update` verify:

1. Manifest signature against a **pinned public key** (build-time or `PVE_ZFS_MONITOR_TRUST_KEY_FILE`).
2. Artifact SHA-256 and size from the manifest.
3. Channel (`stable` vs `lab`) matches the installed binary.

**No production signing key is published in this repository.** CI and development builds are **not trusted releases** (`trusted_release: false` in `version --json`). A maintainer must approve and publish a signing key and signed artifacts on isolated infrastructure before lab/production use.

Private signing keys must never appear in git, PR CI, or downloadable archives.

## Supported architectures

- `linux/amd64`
- `linux/arm64`

Install path: `/usr/local/bin/pve-zfs-monitor`  
Rollback copy: `/usr/local/bin/pve-zfs-monitor.previous`

## One-time install (curl bootstrap)

Obtain the **reviewed** ed25519 public key and an **out-of-band** SHA-256 digest of `install.sh` from the maintainer (not from the release host alone).

```bash
export PVE_ZFS_MONITOR_RELEASE_BASE="https://example.invalid/releases"   # maintainer HTTPS base
export PVE_ZFS_MONITOR_TRUST_KEY_FILE="/path/to/reviewed-signing-key.pub.b64"

curl --proto '=https' --tlsv1.2 --proto-redir '=https' -fsSL \
  -o install.sh "$PVE_ZFS_MONITOR_RELEASE_BASE/scripts/install.sh"
# Maintainer-supplied digest (example):
# sha256sum -c install.sh.sha256

sudo INSTALLER_SHA256="<maintainer-hex-digest>" bash install.sh \
  --channel lab --version 1.0.0-rc1 \
  --release-base "$PVE_ZFS_MONITOR_RELEASE_BASE" \
  --trust-key-file "$PVE_ZFS_MONITOR_TRUST_KEY_FILE"
```

`install.sh` is **self-contained**: embedded Python + OpenSSL verify the signed manifest and artifact SHA-256/size **before** any downloaded bytes are activated. It does not download or execute separate helper scripts. Under `/run/lock/pve-zfs-monitor.lock` it persists `/etc/pve-zfs-monitor/release.conf` and the trust key, then atomically installs the verified binary. The downloaded artifact is never executed during bootstrap.

Offline or in-process installs may use `pve-zfs-monitor install-release` with the same verification rules.

## Version

```bash
pve-zfs-monitor version
pve-zfs-monitor version --json
```

Development builds show `(development; not a trusted release)` and `trusted_release: false`.

## Check for updates

```bash
pve-zfs-monitor update --check
```

After install, release base URL and trust key are read from `/etc/pve-zfs-monitor/release.conf` (no environment variables required in a fresh shell).

## Apply update

```bash
sudo pve-zfs-monitor update
sudo pve-zfs-monitor update --dry-run
```

Rules:

- Never switches `stable` ↔ `lab` automatically.
- Never downgrades silently.
- On any verification failure, the installed binary is unchanged.
- Network I/O only for `update` / `update --check` / `install-release` (not for `status`, `check`, `topology`, `devices`).

## Rollback

```bash
sudo pve-zfs-monitor rollback
```

Restores `/usr/local/bin/pve-zfs-monitor.previous` if present from the last successful install/update.

## Offline install

1. On a trusted build host, obtain `manifest.json`, `manifest.json.sig.json`, and the architecture-specific binary from a **signed** lab/stable release.
2. Copy files to the PVE node.
3. Set `PVE_ZFS_MONITOR_TRUST_KEY_FILE` to the reviewed public key.
4. Run:

```bash
sudo PVE_ZFS_MONITOR_TRUST_KEY_FILE=/path/to/key.pub.b64 \
  pve-zfs-monitor install-release \
  --manifest=/path/to/manifest.json \
  --signature=/path/to/manifest.json.sig.json \
  --binary=/path/to/pve-zfs-monitor-linux-amd64
```

## Manual uninstall

```bash
sudo rm -f /usr/local/bin/pve-zfs-monitor /usr/local/bin/pve-zfs-monitor.previous
```

No services, timers, or ZFS/SMART configuration are modified by install/update.

## Building official metadata locally

```bash
make build VERSION=1.0.0-rc1 CHANNEL=lab TRUSTED=false
```

Trusted release builds require maintainer-controlled signing and `TRUSTED=true` plus `TRUST_KEY_B64=...` only on approved release pipelines.

## Lab distribution (blockers)

Before the first trusted lab artifact:

1. Maintainer generates an ed25519 keypair; **private key stays offline**.
2. Publish **public** key through a reviewed channel (not this PR).
3. Sign manifests on an isolated workflow (not public PR CI).
4. Upload `manifest.json`, `manifest.json.sig.json`, and per-arch binaries to HTTPS storage (`stable/` or `lab/`).
5. Run `docs/p1c-lab-validation.md` on a staging PVE 8/9 node using the install script.

GitHub Actions workflow artifacts are for CI only—not a substitute for signed release downloads.
