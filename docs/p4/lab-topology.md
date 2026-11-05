# P4 lab topology

Disposable Proxmox VE lab for integration validation. **No production hosts.**

## Recommended layout

```text
┌─────────────────────────────────────────────────────────────────┐
│  Lab Proxmox VE 8/9 VM (single node or nested)                  │
│  - Synthetic ZFS pool(s) or file-backed test pools ONLY         │
│  - pve-zfs-monitor binary (lab channel, signed when available)  │
│  - State dir: /var/lib/pve-zfs-monitor-lab/ (0700, dedicated)   │
│  - NO systemd timer enabled unless checklist step explicitly on │
└───────────────┬───────────────────────────────┬───────────────┘
                │ read-only CLI                    │ optional
                ▼                                  ▼
         Local JSON / agent section          ┌──────────────────┐
         under /tmp or file drop             │ Observium Lab VM │
                                             │ (poll only)      │
                                             └──────────────────┘
```

## Network boundaries

| Path | Default in P4 plan | Notes |
|------|-------------------|--------|
| Monitor → local ZFS/SMART | Allowed | Read-only commands only |
| Monitor → lab state dir | Allowed | Private directory on PVE node |
| Monitor → SMTP/sendmail | **Disabled** | Use `FakeTransport` / log sink in automation |
| Observium poller → PVE :36602 | **Disabled by default** | Prefer **agent script over SSH/cron** pushing section to Observium, or manual `observium-agent` invocation documented per test |
| Observium → NMS operators | Lab-only | No production notification targets |

If TCP **36602** is required for a specific Observium version test:

1. Document firewall scope (Observium IP → PVE only).
2. Require **written operator authorization** before enabling listener.
3. Tear down listener after test window.

## Roles

| Role | Responsibility |
|------|----------------|
| **Lab operator** | VM lifecycle, snapshots, rollback, approves network/mail |
| **Executor (agent/CI)** | Runs documented commands, captures logs, no config changes without approval |
| **Reviewer** | Signs off checklist; compares results to acceptance criteria |

## State and rollback

- **Install:** lab-signed binary to `/usr/local/bin/pve-zfs-monitor` per [install.md](../install.md) / [github-pages-lab-install.md](../github-pages-lab-install.md).
- **State:** never use production monitor state paths; lab uses `PVE_ZFS_MONITOR_STATE_DIR` (future config) or documented `--state-dir` once wired.
- **Rollback:** `pve-zfs-monitor rollback`; remove lab state dir; revert Observium PHP copies from git snapshot; disable any temporary agent listener.

## Synthetic vs hardware scenarios

| Scenario | Method |
|----------|--------|
| Degraded mirror, FAULTED member | `internal/zfs/testdata` replay via detection tests; lab optional replay of captured `zpool status` |
| SMART incomplete / stale | Fixture device reports; lab: block `smartctl` or use USB junk disk **only if approved** |
| Crash / corrupt JSON | Fault-injection scripts on lab state dir (see [durability-test-matrix.md](durability-test-matrix.md)) |
| Pool mutation | **Forbidden** — no `zpool destroy`, replace, scrub trigger from monitor |

## Evidence collection

Store under lab operator control (not in git):

- Redacted `state.json` excerpts (hashed keys only—default on disk).
- Observium poller log lines showing app discovery.
- Mail sink logs (null transport / test MTA on lab network only).
