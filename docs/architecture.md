# Architecture and rollout plan

## Goal
Installable Proxmox VE ZFS/NVMe read-only monitoring with optional Observium and independent alerts. Support common ZFS mirror layouts (2 NVMe/RAID1 equivalent), and later arbitrary pool topologies. No storage modification.

## Component boundaries

1. **Collectors:** local fixed-command execution of `zpool`, `zfs`, `smartctl` and/or `nvme` where available. Collect with deadlines and bounded output. Never shell-interpolate device names. Current foundation only collects pool name + health.
2. **Normalized model:** pools, vdevs, member devices, status, errors, last scrub, NVMe temperature, percentage used, spare, media errors and data freshness. Use a stable per-device identity without exposing serial numbers publicly.
3. **Decision engine:** configuration thresholds, fail-closed unknown, rising counter deltas (counter reset/reboot handling), durable per-node dedupe and recovery.
4. **Output adapters:** CLI human/JSON first; Observium Unix Agent and server-side support after validating the platform's expected metric ingestion; Proxmox Notifications via supported system-mail route, optionally standalone SMTP. Prevent duplicate delivery if both alert paths are enabled.
5. **Installer:** reproducible Linux amd64 binary and .deb with opt-in systemd timer; idempotent install/upgrade/remove; no automatic config overwrite and no boot-time storage changes.

## Integration notes

- Observium Unix Agent commonly listens on TCP 36602, but a raw custom local section does **not** guarantee a new graph: test actual Observium application/poller support before claiming graphs or alert rules.
- Check native Observium ZFS/SMART coverage first; avoid duplicating built-in sensors.
- Proxmox ZFS Event Daemon (ZED), smartd and existing system mail should be assessed before inventing a notification transport. Respect Proxmox notification matchers and existing admin alerting.
- Scope privileges to host reading, avoid running persistent root services where possible.

## Phases / acceptance gates

**P0 — Foundation (this PR):** Go module, safe read-only pool health CLI, parser tests, docs and unprivileged public CI. No node install.

**P1 — Collectors:** ZFS vdev topology/errors/scrub and NVMe telemetry; stable device mapping across /dev aliases and namespaces; JSON schemas/fixtures; parser fuzz/negative tests; tolerate missing tools, no pools, permission failure.

**P2 — Detection:** threshold config, read-only periodic checks, stored event fingerprints, reminder throttle, alert and recovery transition tests with simulated clocks, atomic state and no secrets in state.

**P3 — Integrations:** Observium feasibility verification and adapter/probe, Proxmox system-mail adapter and optional SMTP. Integration tests with fake endpoints only; explicit user opt-in, default no real send.

**P4 — Packaging and staging:** .deb, systemd timer with least privileges, upgrade/uninstall verification, Debian/Proxmox VE 8/9 lab results; only then consider a signed release.

## Non-goals for initial releases

No Proxmox web UI plugin, custom listening HTTP server, telemetry service, ZFS repair/autofix, forced scrubs, drive replacement, automatic package install on production nodes, or secret-bearing CI.
