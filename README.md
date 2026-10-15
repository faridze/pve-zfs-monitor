# pve-zfs-monitor

Lightweight Go-based ZFS and NVMe health monitoring for Proxmox VE, with planned Observium integration and independent alert notifications.

> **Early foundation / not production-ready.** NVMe telemetry, notifications, integration, packaging and deployment are not implemented yet. Do not rely on this tool as a replacement for existing ZFS/SMART monitoring.

## Current functionality

- Small read-only Go CLI checking health states of local ZFS pools.
- `status` and `check` commands, with optional `--json`.
- `topology --json` for read-only ZFS vdev topology, error counters and scrub metadata (diagnostic).
- No daemon, no port, no installation hooks, no disk or pool changes.
- Zero third-party Go dependencies.

### Local build

Requires Go 1.23+ to build; running a compiled binary does not require Go.

```sh
go test ./...
go vet ./...
CGO_ENABLED=0 go build -trimpath -o pve-zfs-monitor ./cmd/pve-zfs-monitor
./pve-zfs-monitor version
./pve-zfs-monitor status --json
./pve-zfs-monitor check
```

On Proxmox, the read-only `/usr/sbin/zpool list -H -o name,health` command must be available. Run using an account with sufficient permissions. This initial command checks **pool-level health only**, not mirror member state, SMART attributes, scrub progress or alerts.

Exit codes for `check`: `0` = all reported pools ONLINE, `2` = known unhealthy state, `3` = unknown or collection error. `status` returns success after displaying collected data, even when a pool is degraded; monitoring should use `check`.

## Roadmap

See [architecture and phased delivery](docs/architecture.md) and [agent instructions](AGENTS.md). Later phases target:
1. Individual mirror member mapping, scrub details and NVMe health with fixtures.
2. Observium Unix Agent integration and an explicitly validated ingestion path.
3. Proxmox system-mail notifications and optional standalone SMTP with deduplication.
4. Debian packaging, safe installation and systemd timer, tested on staging Proxmox VE 8/9.

**Safety:** inspect the PR and test on a non-production Proxmox node before any deployment. See [SECURITY.md](SECURITY.md).

Licensed under Apache-2.0.
