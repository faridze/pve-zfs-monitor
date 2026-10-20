# P3: Observium and mail delivery (offline / non-production)

**Status:** library and synthetic tests only. No systemd socket, xinetd, production Observium server changes, or real SMTP.

## Observium integration architecture (verified)

Per [Observium Unix Agent](https://docs.observium.org/unix_agent/):

- SNMP remains required on the device; the agent supplements SNMP via TCP **36602** (or local script invocation).
- The poller pulls all agent modules in one connection into `$agent_data['app'][<name>]`.
- Emitting a custom text block **does not create graphs** until Observium ships a matching `includes/polling/applications/<app>.inc.php`, RRD definitions, and UI graphs ([Adding an Application](https://docs.observium.org/developing/add_app/)).

### Supported approach in this repository

1. **Agent script output (host side):** `internal/observium` renders a single application section `pve_zfs_monitor` with privacy-safe aggregate fields derived from `monitor` signals (hashed keys only, no pool names, serials or paths).
2. **Observium server side (out of repo):** A future `pve_zfs_monitor` application module must be added to Observium to parse the section and define RRD/graphs. This phase delivers the **contract and fixtures** only.
3. **Native overlap:** Prefer Observium built-in ZFS/SMART where sufficient; this app exposes **P2-C decision summary** (worst level, alert/recovery counts) rather than duplicating raw SMART attributes.

### Application wire format

```
<<<pve_zfs_monitor>>>
schema_version=1
collected_at=2026-10-20T12:00:00Z
signals_total=12
signals_healthy=8
signals_warning=1
signals_critical=1
signals_unknown=2
worst_level=critical
events_pending=1
```

Lines are `key=value`; values are bounded integers or enumerated levels. Total output is capped (8 KiB).

## Mail and outbox architecture

P2-B `FileStore.Advance` persists dedupe state but **does not** deliver notifications. P3 adds:

1. **`internal/outbox`:** Append-only durable queue of `monitor.Event` records with delivery state (`pending`, `delivered`, `failed`), atomic same-directory writes, and idempotent keys.
2. **`MailFormatter`:** Proxmox-style plain-text body (subject + body) without embedding raw hardware identifiers.
3. **`Transport` interface:** `Send(ctx, message)`; default tests use `FakeTransport` (in-memory, no network).

Delivery is **opt-in** and disabled by default. A future scheduler calls `outbox.Store.Enqueue` after `Advance` / `DecideWithDetection`, then `DeliverPending` with a configured transport.

## Non-goals (this phase)

- No merge to production nodes, no open port 36602, no real recipients in CI.
- No duplicate ZED kernel-event mail; operators must configure matchers separately.
- No exactly-once guarantee across SMTP; outbox + ack reduces loss but does not replace remote MTA semantics.

## Testing

```bash
go test -count=1 ./internal/observium ./internal/outbox
go test -race ./internal/outbox
```

Synthetic fake SMTP via `FakeTransport` only.
