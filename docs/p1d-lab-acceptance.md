# P1D lab acceptance checklist (non-production PVE 8/9)

Record command output for maintainer review. Redact serial numbers and host-identifying fields before sharing logs.

## 0. Trust material (before any root action)

- [ ] Maintainer `install.sh.sha256` received out-of-band
- [ ] Maintainer `trusted-signing-key.pub.b64` fingerprint verified independently
- [ ] `PVE_ZFS_MONITOR_HERMETIC_TEST` is **unset** on the node

## 1. Independent `install.sh` verification

```bash
sha256sum -c install.sh.sha256    # must pass before sudo
export INSTALLER_SHA256="$(awk '{print $1}' install.sh.sha256)"
```

## 2. One-time curl install (no Git/Go)

```bash
sudo INSTALLER_SHA256="$INSTALLER_SHA256" bash install.sh \
  --channel lab --version 1.0.0-rc1 \
  --release-base "https://<releases>/lab" \
  --trust-key-file /path/to/reviewed-signing-key.pub.b64
```

## 3. Version metadata

```bash
pve-zfs-monitor version --json
```

Expect `channel: lab`, expected SemVer, `trusted_release` per build policy.

## 4. Update check when current

Fresh login shell (no `PVE_ZFS_MONITOR_*` exports):

```bash
pve-zfs-monitor update --check
```

Expect exit `0` and “already up to date” when on latest signed lab manifest.

## 5. Upgrade lab rc1 → rc2

Publish/sign `1.0.0-rc2` separately, then:

```bash
sudo pve-zfs-monitor update
pve-zfs-monitor version --json
```

## 6. Rollback

```bash
sudo pve-zfs-monitor rollback
pve-zfs-monitor version --json
```

Confirm version matches previous binary; config/trust anchor unchanged unless rotation was explicit.

## 7. Read-only collectors

```bash
pve-zfs-monitor topology --json | jq .
pve-zfs-monitor devices --json | jq .
```

Confirm JSON schema; no destructive ZFS/SMART changes.

## 8. Redaction

Before attaching logs:

- Replace disk serials, WWNs, hostnames, and IP addresses with placeholders.
- Keep channel, version, exit codes, and verification messages.

## Key rotation (only if maintainer directed)

See `docs/p1d-lab-key-rotation.md` and use `--allow-trust-key-rotation` / `PVE_ZFS_MONITOR_ALLOW_TRUST_KEY_ROTATION=1`.
