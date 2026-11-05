# Observium lab validation plan

**Scope:** Validate schema v2 `pve_zfs_monitor` application section and optional server-side polling on a **disposable Observium lab**. No production Observium. No default :36602 listener on PVE.

## Preconditions

- P3 library `internal/observium` merged; strict `ParseAppSection` matches poller expectations.
- Experimental PHP: `examples/observium/pve_zfs_monitor.inc.php` — **not lab-verified** until this plan runs.
- Native Observium ZFS/SMART sensors documented; this app exports **aggregate P2-C summary** only ([p3-observium-mail.md](../p3-observium-mail.md)).

## Phase 1 — Offline (Cursor / CI) — already partially done

| ID | Case | Method | Pass criteria |
|----|------|--------|---------------|
| O1 | Schema v2 round-trip | `go test ./internal/observium/...` | Render + parse equality |
| O2 | Verified healthy host | `summary_consistency_test` | `ReportFullyHealthy=1` only with verified host + SMART evidence |
| O3 | Degraded / critical pool | `delivery/scenarios_linux_test` export paths | `report_fully_healthy=0`, critical visible |
| O4 | Incomplete SMART | `export_test`, device evidence tests | `smart_completeness≠complete` or downgraded |
| O5 | Stale observations | `MaxSignalAge` default 10m | `stale_observation=1` |
| O6 | Missing/malformed fields | `agent_test` negative cases | Parser error |
| O7 | Duplicate/unknown keys | Forged section strings | Parser error |
| O8 | False healthy forgery | `summary_consistency_test` | Parser rejects inconsistent `report_fully_healthy` |

## Phase 2 — Lab agent output (no listener)

**Goal:** Produce agent-compatible text on PVE lab without opening port 36602.

1. Run collection + detection (future CLI or one-shot Go harness—**document command when wired**).
2. Call `observium.BuildSummary` + `RenderAppSection`.
3. Write output to `/tmp/pve-zfs-monitor-lab/agent-section.txt` (mode `0600`).
4. Compare against `ParseAppSection` on same host.

| ID | Lab condition | Expected section flags |
|----|---------------|------------------------|
| L-O1 | Healthy synthetic mirror fixture / known-good pool | `worst_level=healthy`, `report_fully_healthy=1` only if SMART evidence verified |
| L-O2 | Degraded pool (documented lab pool or replayed status) | `report_fully_healthy=0`, pool fault/degraded counts > 0 |
| L-O3 | SMART collection skipped | `smart_completeness` incomplete/unknown, not fully healthy |
| L-O4 | Stale `CollectedAt` / old observation | `stale_observation=1` |

## Phase 3 — Observium server (disposable VM)

1. Copy experimental PHP into Observium tree **on lab Observium only** (paths per [Observium add_app](https://docs.observium.org/developing/add_app/)).
2. Register application `pve_zfs_monitor`.
3. **Ingest method (pick one per test run):**
   - **A (preferred):** Paste/upload agent section into poller debug / manual `$agent_data` injection (vendor-specific debug—document actual Observium version).
   - **B:** Run Observium’s poller against a **lab-authorized** agent script on PVE (cron/SSH), still **without** permanent :36602 if avoidable.
   - **C:** Enable :36602 **only with operator authorization**; firewall to Observium IP; disable after test.

| ID | Case | Pass criteria |
|----|------|---------------|
| L-O5 | Application discovery | App appears on device Applications page |
| L-O6 | Poll success | Poller log: no parse errors; metrics stored |
| L-O7 | Poll failure | Inject malformed section → Observium shows error state, **no false green** |
| L-O8 | Degraded signal | UI/RRD reflects non-healthy worst_level (if RRD wired in PHP) |

## Phase 4 — Negative / security

- Reject sections containing pool names, serials, paths (grep lab output).
- Reject oversize section (> 8 KiB) from renderer.
- Document overlap with native Observium ZFS alerts ([alert-routing-policy.md](alert-routing-policy.md)).

## Failure handling

| Failure | Action |
|---------|--------|
| PHP parse mismatch | Fix example PHP or export contract; re-run O1–O8 |
| Poller silent | Do not claim compatibility; keep **experimental** label |
| False healthy in UI | Block lab sign-off; file as P4 defect |

## Deliverables from lab run

- Timestamped poller log excerpt (redacted).
- Screenshot or CLI proof of app status (lab-only storage).
- Checklist rows in [lab-acceptance-checklist.md](lab-acceptance-checklist.md).

**Not in this documentation PR:** enabling listeners, installing PHP on production Observium, or modifying Observium core.
