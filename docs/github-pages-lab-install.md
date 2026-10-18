# GitHub Pages lab install (non-production)

**Release base (no `/lab` suffix):** `https://faridze.github.io/pve-zfs-monitor`

**Status:** Execute the commands below only after the maintainer has published a **signed** `1.0.0-rc1` lab tree to the `gh-pages` branch and you have verified `install.sh.sha256` out-of-band.

## Maintainer publication (one-time per release)

On a trusted machine with the **offline lab signing key** (never in git/CI):

```bash
# 1) Create lab-only key (once) — private key stays local:
openssl genpkey -algorithm ED25519 -out lab-signing.pem
chmod 600 lab-signing.pem
# Export public key (base64 raw 32 bytes) to trusted-signing-key.pub.b64 — distribute fingerprint separately.

export GIT_SHA='b2245e8a28bc393ef31ab343ba2c507c67e1508a'
export VERSION='1.0.0-rc1'
export RELEASE_BASE='https://faridze.github.io/pve-zfs-monitor'
./release/signing/build-lab-artifacts.sh

export PVE_ZFS_MONITOR_SIGNING_KEY_PEM=./lab-signing.pem
./release/signing/sign-manifest-offline.sh dist/lab/$VERSION/manifest.json
./release/signing/verify-bundle-offline.sh dist/lab/$VERSION trusted-signing-key.pub.b64

./release/signing/assemble-gh-pages-tree.sh dist/lab/$VERSION /tmp/gh-pages-out trusted-signing-key.pub.b64
# After explicit approval: push /tmp/gh-pages-out contents to branch gh-pages
```

After HTTPS is live, confirm origin:

```bash
export RELEASE_BASE='https://faridze.github.io/pve-zfs-monitor'
./release/signing/verify-release-origin.sh dist/lab/$VERSION trusted-signing-key.pub.b64 "$RELEASE_BASE"
```

## Operator install (Proxmox — no Git/Go)

**Not executable until signed files are on GitHub Pages and digest is verified.**

```bash
export RELEASE_BASE='https://faridze.github.io/pve-zfs-monitor'
export TRUST_KEY='/path/to/reviewed-trusted-signing-key.pub.b64'

curl --proto '=https' --tlsv1.2 --proto-redir '=https' -fsSL \
  -o install.sh.sha256 "$RELEASE_BASE/scripts/install.sh.sha256"
curl --proto '=https' --tlsv1.2 --proto-redir '=https' -fsSL \
  -o install.sh "$RELEASE_BASE/scripts/install.sh"
sha256sum -c install.sh.sha256

export INSTALLER_SHA256="$(awk '{print $1}' install.sh.sha256)"
sudo INSTALLER_SHA256="$INSTALLER_SHA256" bash install.sh \
  --channel lab --version 1.0.0-rc1 \
  --release-base "$RELEASE_BASE" \
  --trust-key-file "$TRUST_KEY"
```

## After install

```bash
pve-zfs-monitor version
pve-zfs-monitor status
pve-zfs-monitor check
pve-zfs-monitor topology --json
pve-zfs-monitor devices --json
pve-zfs-monitor update --check
sudo pve-zfs-monitor update
```

`version --json` reports `trusted_release: false` and `development: true` for standard lab builds (expected; integrity comes from signed manifests, not the ldflag).
