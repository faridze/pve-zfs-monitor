# P3: Observium export, durable outbox, and mail transport

**Status (offline):** Implemented and covered by synthetic Go tests. **Not verified** against a live Observium poller, Proxmox notification matcher, or real MTA.

## A — Observium (`internal/observium`)

### Native Observium overlap

Observium already ships ZFS pool monitoring and SMART/NVMe-oriented checks via SNMP and other applications. This project does **not** duplicate raw SMART attributes, serials, WWNs, pool names, or device paths. The `pve_zfs_monitor` application section exports **P2-C decision aggregates** only: host overall level, pool-scoped level counts, signal level counts, SMART collection completeness, collection failure flag, data freshness (`stale_observation`), verified critical count, worst level, pending notification count, and `report_fully_healthy` (fail-closed).

### Unix Agent contract (schema v2)

Wire format: `<<<pve_zfs_monitor>>>` header, then fixed `key=value` lines (max ~8 KiB). Required keys are enumerated in `internal/observium/agent.go`. `ParseAppSection` is a strict fake consumer: rejects duplicate, missing, unknown, malformed integers, inconsistent totals, and inconsistent `worst_level` / `report_fully_healthy`.

Empty, stale, unverified, partially collected, or incomplete SMART snapshots **never** set `report_fully_healthy=1`. Independently verified critical signals keep `worst_level=critical` even when other signals are unknown.

### Server-side Observium (not in repo)

Per [Unix Agent](https://docs.observium.org/unix_agent/) and [Adding an Application](https://docs.observium.org/developing/add_app/):

1. Agent output alone does not create graphs or alert rules.
2. Operators need `includes/polling/applications/pve_zfs_monitor.inc.php`, RRD definitions, and UI graph pages.
3. Experimental reference PHP lives under `examples/observium/` with synthetic fixtures only — **not lab-verified**.

Do not open port 36602 or configure xinetd/systemd from this repository.

## B — Atomic outbox (`internal/delivery` + `internal/outbox`)

P2-B `FileStore.Advance()` persisted dedupe state only. P3 combines monitor state and notification outbox in one `state.json` document (`format_version: 2`). Legacy v1 state files migrate on read (empty outbox).

`delivery.Coordinator.AdvanceDetection()` holds an exclusive lock, runs `DecideWithDetection`, enqueues events with dedupe fingerprints, and **atomically** writes state + outbox (rename + fsync). There is no supported path that calls `Advance` and `Enqueue` as separate durable steps.

`DeliverNotifications()` delivers pending and **failed-but-retryable** entries (bounded exponential backoff), then persists acknowledgments in the same locked write.

**Semantics:** durable notification **intent** with **at-least-once** external delivery. If the process crashes after transport success but before the durable ack is written, a retry may produce a duplicate external message. Exactly-once is not claimed.

**Ordering:** deliverable entries sort by severity (Critical alert first, then Warning, Unknown, Reminder, Recovery) then age, so a pending Critical is sent before a later Recovery.

**Privacy:** queue entries store hashed signal keys only; no recipients, credentials, or raw hardware identifiers on disk.

Linux implementation: `internal/delivery/persist_linux.go` (private `0700` dir, path traversal checks, `flock`, atomic replace). Non-Linux builds expose a stub coordinator for compile-only CI.

## C — Mail transport (`internal/outbox`)

- `Transport` interface with `Send(ctx, message)` — opt-in, no default production transport.
- `FakeTransport` for all automated tests (no network, no shell).
- `FormatMessage` / `FormatProxmoxStyle`: Critical, Warning, Unknown, Reminder, Recovery plain-text bodies; bounded size; hashed keys only.
- `SystemMailTransport` in `systemmail_stub.go`: **disabled prototype** (fixed executable path, argument array, deadline) — not validated on a PVE lab; do not enable in CI.

### Duplicate alerts with ZED / Observium / this monitor

ZED, Observium, and pve-zfs-monitor can all surface storage problems through different channels. Operators should pick **one authoritative route** per severity class (for example: ZED for immediate kernel events, this monitor for verified policy, Observium for dashboards) and disable or filter overlapping matchers elsewhere. This project does not modify ZED, smartd, Postfix, Exim, or Proxmox notification matchers.

## Testing

```bash
gofmt -w ./internal ./cmd
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o /tmp/pve-zfs-monitor ./cmd/pve-zfs-monitor
```

Linux-only integration: `internal/delivery/*_linux_test.go` (restart survival, legacy migration, corrupt document, concurrent lock, topology scenarios).

## PR scope note

P3-A/B/C are delivered together on Draft PR #14 (`cursor/p3-observium-mail-outbox-8c2d`). A stacked split (P3-B on top of P3-A) is possible for review but not required for this milestone.
