# P4-B Lab orchestration CLI

PROMPT-ID: `PVE-ZFS-P4-B-LAB-ORCHESTRATION-20261104-001`

Lab-only, single-run wiring of read-only collectors, P2 detection, and P3 durable state/outbox. **Not** for production scheduling, real mail, or Observium polling on this path.

## Command syntax

```bash
pve-zfs-monitor monitor run-once \
  --lab \
  --state-dir /absolute/lab/private/state \
  --delivery none \
  --json
```

| Flag | Required | Values | Meaning |
|------|----------|--------|---------|
| `--lab` | yes | (flag) | Explicit lab mode; command refuses to run without it |
| `--state-dir` | yes | clean absolute path | Pre-created directory, mode `0700`, owned by invoking user |
| `--delivery` | no (default `none`) | `none` \| `fake` | `none`: enqueue only; `fake`: lab simulated ack via `FakeTransport` |
| `--json` | no | (flag) | Privacy-safe bounded JSON report on stdout |

Human-readable output is used when `--json` is omitted.

## State directory preparation

```bash
install -d -m 0700 /var/lib/pve-zfs-monitor-lab/state
# Do not reuse production paths. Do not chmod production directories from this doc.
```

The store validates:

- Path is absolute and clean (no `.` / `..` segments).
- Directory exists before the run (the CLI does **not** create lab state roots).
- Mode `0700` and owner matches process EUID (same rules as `internal/delivery`).

Files created inside the directory (by the durable store): `state.json`, `state.lock`, `dispatch.lock`.

## Exit codes

| Code | Meaning |
|------|---------|
| 0 | Completed; overall status is Healthy or Warning |
| 2 | Completed; overall verified Critical |
| 3 | Persistence/detection failure or overall Unknown |
| 64 | Usage / invalid flags / unsafe state path |

## JSON report fields

- `overall`, `verified_critical`, `verified_warning`, `unknown_signals`, `healthy_signals`
- `events_emitted` (key/kind/level/emitted_unix only — no serials or device paths)
- `pending_notifications`, `simulated_deliveries`, `simulated_acknowledgment`
- `collection_failures` (aggregate collector errors)
- `state_persisted`
- `at_least_once_limitation` (present when `--delivery fake`)

## Simulated delivery (`--delivery fake`)

- Uses `outbox.FakeTransport` only; no sendmail, SMTP, webhooks, or network I/O.
- Acknowledgments are labeled via `simulated_acknowledgment` and delivery counts.
- Documented limitation: **at-least-once** — crash after simulated send but before durable ack may produce duplicate simulated deliveries on retry.

## Limitations (NOT VERIFIED in this PR)

- Real Observium agent polling and PHP poller integration on a live host.
- System mail / SMTP production transport.
- Background timer, daemon, or installer changes.
- ZFS or disk mutations (still read-only collectors only).

## Rollback

1. Stop invoking `monitor run-once` in lab scripts.
2. Remove or archive the lab `--state-dir` tree (contains monitor state and outbox snapshot only).
3. Revert to prior binary; no pool or disk operations are required.

## Example lab execution

```bash
# Build (lab host or CI artifact)
CGO_ENABLED=0 go build -trimpath -o /tmp/pve-zfs-monitor ./cmd/pve-zfs-monitor

install -d -m 0700 /var/lib/pve-zfs-monitor-lab/state

/tmp/pve-zfs-monitor monitor run-once \
  --lab \
  --state-dir /var/lib/pve-zfs-monitor-lab/state \
  --delivery none \
  --json

# Optional simulated delivery pass (same state dir, after events queued):
/tmp/pve-zfs-monitor monitor run-once \
  --lab \
  --state-dir /var/lib/pve-zfs-monitor-lab/state \
  --delivery fake \
  --json
```

Each invocation is a **single** cycle; there is no built-in scheduler.
