# P2-A: offline alert-decision readiness (no delivery)

**Status:** experimental library, synthetic CI only. No production deployment,
scheduler, state-file writer, Observium export or mail transport.

## Why separate detection from delivery

PVE pool and SMART collection already work as read-only diagnostics. Running a
poll every N minutes is **not** equivalent to safe alerting: missing telemetry,
timeout, parser uncertainty and failure/recovery flapping must not turn into
false OK or repeated emails.

The new `internal/monitor` package implements only an *offline*, deterministic
transition evaluator. It never invokes `zpool`, `smartctl`, an SMTP client,
Observium, systemd, or disk I/O. This PR makes no changes to existing CLI
commands and has no background service.

## Interface and safe defaults

- `PoolSignals` transforms provided `zpool list` results into normalized
  signals; known ZFS health states are categorized, unfamiliar states are
  **UNKNOWN**. No hidden hardware reads.
- Keys are scoped SHA-256 digests of identifier strings. State/events contain
  no cleartext pool names, device paths, serials or other raw telemetry.
- `Decide(previous, signals, time, options)` is pure. It returns a new
  versioned in-memory state and an event list; callers must explicitly store
  or deliver either. No persistent state is implemented here.
- Default: observations older than 10 minutes (or from the future) are
  **UNKNOWN**. Unverified observations can never clear an alert.
- First observed degraded/failed/unknown signal creates one alert; further
  identical observations are deduplicated. A reminder may be produced after
  24 hours. **Two consecutive fresh verified healthy observations** confirm
  recovery. Absent previously-seen signals become UNKNOWN.
- Non-monotonic clocks, malformed keys, duplicate signals, invalid state
  versions and more than 512 signals fail without state mutation.
- With zero observations and zero prior state, a synthetic host-level
  **UNKNOWN** is emitted rather than a healthy baseline.
- The caller remains responsible for collecting a *complete* observation
  snapshot. For now P2-A only covers pool-level states. It does not infer
  member health, scrubs or SMART thresholds from these pool signals.

## Synthetic tests

Run on GitHub-hosted public CI only:

```bash
go test -count=1 -v ./internal/monitor
go test -race ./internal/monitor
```

The tests exercise HEALTHY → DEGRADED → reminder → confirmed recovery,
FAILED → stale UNKNOWN → verified recovery, failed collection, missing signals,
clock regressions, malformed state, persisted **JSON serialization roundtrip**
(in memory), and privacy. “Email” and “Observium” in the fixture tests are
two in-memory slices, **not protocol adapters or actual delivery**.

## Explicit gates remaining

1. **P2-B: durable, atomic dedupe state** (single-writer lock, root-owned
   0600 storage, no symlink traversal, recovery from corrupt state, and
   fail-closed no delivery before persistence). Test file crash cases.
2. **P2-C: complete detector** for pool topology, mirror-member errors and
   SMART. Preserve unknown on missing/partial/stale device reports. Introduce
   validated and opt-in thresholds per model; do not assume 56°C is unsafe.
3. **P3-Observium feasibility:** inspect existing Observium native
   SMART/ZFS support, choose a supported Unix Agent or poller ingestion
   contract, and test with a fake poller first. A random custom Unix Agent
   section alone does not guarantee dashboard graphs or alert rules.
4. **P3-Mail feasibility:** assess Proxmox ZED/notification matchers and
   system-mail behavior before designing sender. Test with local fake
   SMTP/mail collector only, never real addresses or credentials in CI.
5. **P4:** opt-in interval schedule, least-privilege execution, bounded runtime,
   bounded output, safe throttling, package/install/restart/rollback in a
   non-production PVE 8/9 lab. No automatic scheduling or outbound delivery
   in this PR.

**No action is needed on any operational Proxmox host.** Security constraints
and no-external-effects policy remain in force.
