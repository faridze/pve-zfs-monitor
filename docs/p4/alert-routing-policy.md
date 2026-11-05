# Alert routing and duplicate-prevention policy (lab)

**Purpose:** Avoid duplicate storage alerts from ZED, Observium, and pve-zfs-monitor without modifying any service configuration in this documentation PR.

## Alert sources (typical Proxmox + Observium)

| Source | What it watches | Typical channel |
|--------|-----------------|-----------------|
| **ZED (ZFS Event Daemon)** | Kernel/ZFS events (pool degrade, device faults) | Email/script per ZED config |
| **Observium** | SNMP + applications (ZFS, SMART, custom apps) | NMS alerts, email, Slack (site config) |
| **pve-zfs-monitor** | Read-only aggregated policy (P2-C), optional outbox mail | Opt-in transport (future CLI) |

These layers are **complementary but overlapping**—the same pool degrade may appear in all three.

## Recommended authoritative policy (choose explicitly)

Document the lab operator’s choice before enabling more than one outbound path:

| Tier | Suggested authoritative channel | Others |
|------|--------------------------------|--------|
| **Immediate kernel/ZFS events** | ZED **or** monitor Critical — **not both to email** | Observium = visual only |
| **Policy / SMART / scrub completeness** | pve-zfs-monitor | Observium native SMART optional |
| **Dashboards / history** | Observium | Disable duplicate Observium alert rules for same condition |
| **Verified recovery** | Single channel (monitor **or** ticket system) | Suppress reminder spam elsewhere |

## Duplicate-prevention mechanisms in this project

| Mechanism | Scope |
|-----------|--------|
| Monitor dedupe + recovery samples | Same signal key; no repeat alert until recovery |
| Outbox `EmittedUnix` fingerprint | Same incident vs new incident after recovery |
| Delivered compaction | Old delivered rows pruned; does not delete pending Critical |
| `dispatch.lock` | Prevents double send under concurrent dispatchers |
| At-least-once after crash-after-send | **Allows duplicate external mail** — document to operators |

## What this project will **not** do (P4 plan)

- Edit `/etc/zfs/zed.d`, `smartd.conf`, Postfix, or Proxmox notification matchers automatically.
- Disable ZED or Observium alerts programmatically.
- Merge Observium alerts with monitor outbox without operator config.

## Lab documentation task

Before lab mail/Observium tests, record:

1. Which channel is **authoritative** for Critical pool events.
2. Which channels are **monitoring-only** (no email).
3. Contact/alias used for lab mail sink (if any).

## Observium-specific

- Prefer **one** of: native ZFS health sensors **or** `pve_zfs_monitor` app for alerting—not both firing on same condition.
- Export schema is aggregate; use Observium alert rules on `worst_level`, `report_fully_healthy`, `verified_critical_signals` — not on raw SMART attributes duplicated elsewhere.

## Sign-off question

> “If a pool goes DEGRADED, how many emails should the lab receive?”  

Target for a well-configured lab: **one** intentional notification per policy, plus documented at-least-once duplicates only during fault-injection tests (D5).
