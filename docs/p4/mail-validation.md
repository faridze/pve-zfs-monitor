# Proxmox mail lab validation plan

**Scope:** Validate notification **formatting**, **outbox delivery semantics**, and (later) **SystemMailTransport** on a disposable PVE lab. **Real email disabled** during Cursor automation; lab mail only with explicit operator approval.

## Current code status

| Piece | Status |
|-------|--------|
| `outbox.FormatMessage` | Implemented; Alert/Reminder/Recovery; hashed keys |
| `outbox.FakeTransport` | Implemented; used in all CI delivery tests |
| `SystemMailTransport` | **Disabled prototype** — returns error even if `Enabled=true` ([systemmail_stub.go](../../internal/outbox/systemmail_stub.go)) |
| CLI / config for recipients | **Not implemented** |
| Proxmox notification matcher integration | **Not implemented** — do not modify Proxmox matchers in P4 doc phase |

## Phase 1 — Automated (Cursor Cloud)

| ID | Case | Command / test | Pass |
|----|------|----------------|------|
| M1 | Critical alert body | `go test ./internal/outbox/...` | Subject/body bounded; no `/dev/`, no `@` in fixtures |
| M2 | Reminder / Recovery formats | `mail_test.go` | All kinds formatted |
| M3 | FakeTransport delivery | `store_test`, `integration_test` | Messages recorded; state → delivered |
| M4 | Transport error redaction | `lifecycle_test` | No email-like secrets in `LastError` |
| M5 | Retry after failure | `queue_test`, `scenarios_linux_test` | Failed → backoff → success |
| M6 | Crash before ACK | `queue_test` AtLeastOnce | Second send allowed; documented duplicate risk |

## Phase 2 — Lab procedure (transport = log sink, not SMTP)

Until `SystemMailTransport` is implemented:

1. Configure lab harness to use **`FakeTransport`** or a **LogTransport** (future) writing to `/var/lib/pve-zfs-monitor-lab/mail.log`.
2. Run delivery coordinator with lab state dir.
3. Verify message count matches pending intents; verify no duplicate sends under concurrent dispatch (`ErrDispatchBusy`).

| ID | Event kind | Verify |
|----|------------|--------|
| L-M1 | Critical alert | One log entry; hashed key in body |
| L-M2 | Reminder | Throttled by monitor policy; dedupe in outbox |
| L-M3 | Recovery | Delivered after two-sample healthy transition (monitor config) |
| L-M4 | Concurrent dispatchers | Single external delivery per intent |

## Phase 3 — System mail (future lab — blocked on implementation)

**Blocker:** `SystemMailTransport.Send` must be implemented with:

- Fixed executable path (e.g. `/usr/sbin/sendmail`) — **array argv**, no shell.
- Stdin-only message injection; size limits.
- `context` deadline (match `outbox.TransportTimeout` policy).
- Recipient from **config allowlist** (lab: `root@localhost` or null sink alias only).
- Never log recipient secrets or full message to world-readable files.

| ID | Requirement | Lab check |
|----|-------------|-----------|
| S1 | Disabled by default | Binary/config without flag → no send |
| S2 | Timeout | Hung sendmail → failed status, retryable |
| S3 | Invalid recipient | Rejected before exec |
| S4 | Crash after send before ACK | At-least-once duplicate possible; verify in mail log |
| S5 | Proxmox overlap | Document only — see [alert-routing-policy.md](alert-routing-policy.md) |

**Operator rule:** Do not point lab transport at external MX or real user mailboxes without written approval.

## Proxmox notification configuration

- This project **must not** auto-edit `/etc/pve/notifications`, postfix, or ZED mail in P4 planning.
- Lab doc: record **current** PVE notification settings before any test (screenshot/redacted copy, operator-held).
- If later integrating with Proxmox matchers, require separate design PR + approval.

## Pass/fail (mail track)

- **Pass (Phase 1–2):** All M* tests green; lab log sink shows correct cardinality and redaction.
- **Pass (Phase 3):** Only after stub replaced with reviewed implementation and S* checks signed off.
- **Fail:** Any real mail to production addresses; any credential in state file; duplicate dispatch under concurrency.
