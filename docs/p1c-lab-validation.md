# P1C read-only lab validation checklist (Proxmox VE 8/9)

Use on a **non-production** staging node. Do not run against production pools or during maintenance windows.

## Prerequisites

- `pve-zfs-monitor` binary built from Draft PR #5 HEAD
- Account with `zpool status` read access (root or `zfs` group as applicable)
- Optional: `smartmontools`, `nvme-cli` packages as on standard PVE installs

## Inventory and mapping

1. `pve-zfs-monitor topology --json` — all pools present; list comparison status documented.
2. `pve-zfs-monitor devices --json` — `inventory_completeness`, `mapping_completeness`, `smart_completeness` fields populated.
3. Verify NVMe mirror with partition members (`/dev/nvme*n*p*`) maps to correct namespace/disk.
4. Verify SATA/SAS RAIDZ pool maps `sd*` members without inventing transport when `lsblk TRAN` absent.
5. Rename test: note `runtime_locator` vs `stable_hardware_id` (WWN/by-id) after reboot or NVMe renumber.

## SMART collection (read-only)

1. Run `devices --json` as root and as unprivileged user; confirm `permission_denied` vs `collected` classification.
2. Compare `smart_passed`, `tool_exit_code`, and `diagnostic_flags` with manual `smartctl --json=o -a -d auto <dev>`.
3. Confirm no smartctl self-tests or writes (`-t`, `-s`, etc.) are invoked by the monitor.

## Layout matrix (minimum)

- Single-disk pool
- Mirror (2+ members), including by-id member names
- RAIDZ1/2 pool with SATA/SAS
- Degraded / UNAVAIL GUID member (mapping `missing`, no guessed disk)
- Pool with hot spares section (topology only; spares not SMART-targeted unless mapped)

## Evidence to capture

- Redacted JSON outputs (no serials in public reports)
- `smartctl` exit codes for failing vs healthy devices
- Notes on any `health_conflict` or `partial` collection statuses

## Out of scope for lab pass

- Alerting thresholds, SMTP, Observium export, packaging/systemd install
