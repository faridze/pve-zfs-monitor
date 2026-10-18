# Lab trust-key rotation (maintainer + operator)

## When required

If `/etc/pve-zfs-monitor/trusted-signing-key.pub.b64` already exists and an install supplies a **different** Ed25519 public key with the same `channel` and `release_base`, installation **fails closed** by default. This prevents silent trust-anchor changes that would break `pve-zfs-monitor update`.

## Reviewed rotation procedure

1. Maintainer publishes the new public key fingerprint out-of-band (see `release/signing/fingerprint-pubkey.sh`).
2. Operator records the rotation window and verifies the new key through an independent channel.
3. Re-run installation with an explicit rotation flag:
   - **curl installer:** `sudo PVE_ZFS_MONITOR_ALLOW_TRUST_KEY_ROTATION=1 ... bash install.sh --allow-trust-key-rotation ...`
   - **install-release:** `sudo pve-zfs-monitor install-release ... --allow-trust-key-rotation`
4. Confirm `LoadPersistedConfig` and `update --check` succeed in a fresh shell.

Ordinary version upgrades on the **same** signing key do not require rotation flags.
