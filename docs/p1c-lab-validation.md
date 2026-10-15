# P1C read-only lab validation (Proxmox VE 8 / 9)

Strictly **non-production** staging nodes only. No pool changes, no package installs, no SMART self-tests, no host configuration edits.

## Safety rules

- Do not run on production pools or during maintenance windows.
- Use read-only commands only (`zpool status`, `zpool list`, `lsblk`, `smartctl --json=o` with health args, `stat`, `readlink`).
- Do not use `smartctl -t`, `-s`, `-W`, or any write-capable flags.
- Redact serial numbers and raw SMART JSON before sharing artifacts publicly.

## Build and deploy (lab node)

```bash
# On a build host (not necessarily the PVE node)
git clone https://github.com/faridze/pve-zfs-monitor.git
cd pve-zfs-monitor
git checkout feature/p1c-device-identity-smart
CGO_ENABLED=0 go build -o pve-zfs-monitor ./cmd/pve-zfs-monitor
# Copy binary to lab node (scp/rsync); do not install packages on PVE unless already present.
```

Record binary version: `pve-zfs-monitor version` and git HEAD SHA.

## Procedure overview

| Step | Goal | Pass criteria |
|------|------|----------------|
| A | Topology vs ZFS | All pools/members visible; list comparison documented |
| B | Inventory vs hardware | `inventory_completeness` matches lsblk/by-id reality |
| C | Member mapping | `mapping_completeness`; partition → parent; no phantom paths |
| D | Transport IDs | NVMe/SATA/SAS fields sane; stable vs runtime locator |
| E | SMART vs smartctl | Mapped disks match manual smartctl (redacted) |
| F | Live block identity | major:minor on `/dev` vs sysfs (manual spot-check) |
| G | Permissions | Unprivileged run shows `permission_denied` where expected |
| H | Coverage semantics | Missing/unmapped members ⇒ `smart_incomplete` |
| I | Multi-pool / mixed | Each pool represented; unknown vdevs listed not dropped |

## A — Topology versus membership

```bash
pve-zfs-monitor topology --json | jq '.pools[] | {name, state, list_comparison_status}'
zpool status -v
zpool list
```

Verify every imported pool appears once. Compare leaf member names (paths, by-id, GUIDs) to `zpool status -v`. Note `list_comparison_status` and any `report_issues`.

## B — Inventory versus physical devices

```bash
pve-zfs-monitor devices --json | jq '{inventory_completeness, inventory_issues, inventory_count: (.inventory|length)}'
lsblk -o NAME,PATH,TYPE,SIZE,MODEL,SERIAL,WWN,TRAN,MOUNTPOINT -J
ls -l /dev/disk/by-id/ | head
```

Pass: inventory devices match lsblk block nodes; `inventory_incomplete` when by-id is missing, stale, or conflicting (see `inventory_issues`).

## C — Mapping and partition parents

```bash
pve-zfs-monitor devices --json | jq '.member_mappings[] | {zfs_member_name, outcome, device_path, reason}'
```

Checklist:

- Mirror with `/dev/nvme*n*p*` members maps to parent namespace/disk (`/dev/nvme0n1`), not only partition path in SMART target.
- RAIDZ `sd*` members map without inventing transport when `TRAN` absent.
- Degraded pool with GUID `UNAVAIL` member: `outcome=missing`, no guessed disk.
- **Coverage:** if any member is `missing`, `ambiguous`, or `unsupported`, `mapping_completeness` must be `mapping_incomplete`.

## D — NVMe / SATA / SAS identification

From `devices --json` inventory entries:

- `runtime_locator` reflects current `/dev/...` path.
- `stable_hardware_id` uses WWN/by-id when present; must not treat `/dev/nvmeXnY` alone as stable hardware ID.
- Multiple namespaces on one controller remain distinct entries.

Optional cross-check (read-only): `nvme list` / `nvme id-ctrl` if already installed; do not install packages for this test.

## E — SMART versus native smartctl

For each **mapped** disk path `DEV` (use parent disk/namespace, not partition, when report shows parent):

```bash
# Monitor (root)
pve-zfs-monitor devices --json | jq '.device_health[] | {device_path, collection_status, smart_passed, tool_exit_code, diagnostic_flags}'

# Manual reference (same DEV, read-only)
smartctl --json=o -a -d auto "$DEV"
```

Compare `smart_passed`, temperature, NVMe percentage_used, and exit/diagnostic flags. Unprivileged:

```bash
pve-zfs-monitor devices --json | jq '.device_health[] | select(.collection_status=="permission_denied")'
```

Pass: permission errors classified as `permission_denied`, not generic `failed` when stderr indicates access denial.

## F — Live device major:minor (spot-check)

Pick one mapped NVMe namespace and one SATA/SAS disk:

```bash
DEV=/dev/nvme0n1   # example
readlink -f "$DEV"
stat -c '%t:%T' "$DEV"
cat /sys/class/block/$(basename "$DEV")/dev
```

Pass: block special device; `stat` major:minor matches sysfs `dev` file.  
**Limitation:** the monitor compares live node vs sysfs but does not yet cross-check lsblk-reported major:minor (TOCTOU); lab should document any mismatch.

## G — Report completeness with missing devices

On a node with a degraded mirror or intentionally incomplete inventory (e.g. one disk removed from OS but still in ZFS config):

```bash
pve-zfs-monitor devices --json | jq '{mapping_completeness, smart_completeness, members: [.member_mappings[]|{name: .zfs_member_name, outcome}]}'
```

Pass:

- `mapping_incomplete` when any member not `mapped`.
- `smart_incomplete` when mapping is incomplete **even if** the remaining mapped disk shows `collection_status=collected`.

## H — Multi-pool and mixed layouts

```bash
pve-zfs-monitor devices --json | jq '[.member_mappings[] | .pool_name] | unique'
```

Minimum matrix (use what exists on the lab node):

| Layout | What to verify |
|--------|----------------|
| Single-disk pool | One mapped member; SMART if permitted |
| Mirror (by-id names) | Full by-id path resolution |
| RAIDZ1/2 | All data members in mappings |
| Aux logs/cache/special | Section nodes not treated as data leaves; spares not SMART-targeted unless mapped |
| Unknown terminal vdev (e.g. dRAID) | Present as `unsupported` with reason; mapping incomplete |

## I — Unknown / future vdev types

If `zpool status` shows a vdev class the tool labels `unknown` at a **terminal** leaf (no children), confirm it appears in `member_mappings` with `outcome=unsupported` and does not receive SMART collection.

## Evidence to archive (internal)

- Redacted `topology.json`, `devices.json` snippets (completeness fields + outcomes only).
- Table of DEV → smartctl exit code vs monitor `tool_exit_code`.
- Notes on `health_conflict`, `partial`, `not_attempted`, or context timeout on large pools.
- PVE major version (`pveversion`) and kernel (`uname -r`).

## Out of scope

- Alerting, packaging, systemd install, merge/production deploy.
- Automatic repair, scrub/resilver triggers, SMART self-tests.

## Known monitor limitations (document in lab report)

- Live verification: `/dev` block-special + sysfs major:minor; no lsblk major:minor cross-check yet.
- Non-Linux hosts: live verifier unavailable.
- `devices --json` budget: 120 seconds; very large pools may end with `not_attempted` / `smart_incomplete`.
