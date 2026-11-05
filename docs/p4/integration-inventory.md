# P4 integration inventory

**PROMPT-ID:** PVE-ZFS-P4-LAB-INTEGRATION-PLAN-20261103-001  
**Baseline:** `main` @ `13ae90e27a1f22d5f98373bec0ef830747fa0f9b` (P3 merged via PR #14)

Legend: **Implemented** = shipped Go/library code with tests. **Experimental** = example or disabled stub. **Not wired** = no CLI/installer path yet. **Lab-only** = requires disposable VM and operator approval.

| Component | Implementation | Automated tests | Missing wiring | Lab dependencies | Privileges | Security notes | Lab verification |
|-----------|----------------|-----------------|----------------|------------------|------------|----------------|------------------|
| **CLI `status` / `check`** | Implemented | `cmd/pve-zfs-monitor/*_test.go`, `internal/zfs` | None for pool-level check | PVE with `zpool` | Read ZFS (often root) | Read-only; bounded timeout | Run on lab VM; confirm exit codes |
| **CLI `topology` / `devices`** | Implemented | `internal/zfs`, `internal/device` fixtures | Not fed into monitor loop in CLI | `zpool`, `smartctl`/`nvme` | Root or device group | No shell interpolation; fixture-backed | Compare JSON to synthetic baselines |
| **`internal/monitor` decision** | Implemented (P2-C) | Extensive `*_test.go` | Invoked via **`monitor run-once --lab`** (P4-B) | None for unit tests | N/A in CI | Fail-closed UNKNOWN | Lab: single-run CLI on disposable VM |
| **`internal/observium` export** | Implemented schema v2 | `agent_test`, `export_test`, `summary_consistency_test` | **No CLI subcommand**; no agent script in repo | Observium poller (optional) | N/A for export API | No PII in section; strict parser | Lab: render section from detection output |
| **`examples/observium/*.php`** | Experimental stub | None (not PHP-tested in CI) | Not installed by installer | Observium CE/EE lab instance | Observium admin | Do not deploy to production Observium without review | Manual poller test with fixture agent output |
| **Unix Agent listener :36602** | **Not implemented** (by design) | N/A | No xinetd/systemd socket in repo | Observium + optional agent daemon | Network if enabled | **Do not enable without authorization** | Document-only unless operator opens port |
| **`internal/delivery` + outbox** | Implemented (Linux durable) | `integration_linux_test`, `concurrent_linux_test`, `document_*` | **`monitor run-once --lab`** (P4-B); non-Linux stub | Linux lab VM, private state dir | User-owned `0700` dir | `flock`, atomic rename, dispatch lock | Fault-injection matrix (Gate D) |
| **`internal/outbox` + `FakeTransport`** | Implemented | `queue_test`, `lifecycle_test`, `mail_test` | No production transport default | None | N/A | Hashed keys; redacted transport errors | Automated only in CI |
| **`SystemMailTransport`** | Disabled prototype | Compile-only | `Send` always errors when “enabled” | PVE sendmail/postfix path | root if ever enabled | Fixed path only; no CI execution | Lab Gate C after explicit enablement design |
| **`internal/persistjson`** | Implemented | `strict_test`, document tests | Used on load/save only | None | N/A | Rejects duplicate keys, typos, trailing JSON | Corrupt-file tests already in CI |
| **Installer / updater (P1D)** | Implemented scripts + CLI | `internal/release`, signing scripts | Trusted release not published from dev CI | HTTPS release base, trust key | root for install | Manifest signature, no keys in git | Lab install on disposable VM only |
| **systemd timer / auto poll** | **Not implemented** | N/A | Roadmap P4 packaging | systemd (future) | TBD | Must remain opt-in | **Out of scope** until separate approved PR |

## Entrypoints today

| Entrypoint | Delivers |
|------------|----------|
| `pve-zfs-monitor status\|check\|topology\|devices` | Raw/collected telemetry only |
| `pve-zfs-monitor monitor run-once --lab …` | Single-run collect → detect → durable state/outbox (P4-B) |
| `pve-zfs-monitor version\|update\|rollback\|install-release` | Binary lifecycle (signed release path) |
| Go packages `monitor`, `observium`, `delivery`, `outbox` | Libraries for future daemon/CLI integration |
| `examples/observium/pve_zfs_monitor.inc.php` | Reference poller only |

## Critical gaps before “lab functional”

1. ~~**Orchestration CLI or timer**~~ **P4-B:** `monitor run-once --lab` (single-run; no timer).
2. **Observium path:** script or cron on node to emit agent section (stdout file or agent-compatible output)—without opening :36602 unless approved.
3. **Observium server:** install and validate PHP application module (experimental example is starting point only).
4. **Mail path:** implement and lab-test `SystemMailTransport` behind explicit config (currently stub).
5. **Configuration file** for lab state directory, transport enablement, recipient allowlist (not in repo yet).

## Security summary

- All collection remains read-only; lab must use **non-production** VMs and **separate state directory** (e.g. `/var/lib/pve-zfs-monitor-lab`).
- No real recipients, SMTP credentials, or internal hostnames in git or test artifacts.
- Observium and mail integrations are **opt-in**; default lab procedure keeps notifications disabled (`FakeTransport` or dry-run).

See also: [lab-topology.md](lab-topology.md), [lab-acceptance-checklist.md](lab-acceptance-checklist.md).
