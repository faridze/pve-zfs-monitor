# Agent instructions — pve-zfs-monitor

## Mission
Build a small, reliable, open-source Go health monitor for Proxmox VE ZFS pools and NVMe devices. Read docs/architecture.md first and preserve the staged roadmap.

## Collaboration
- Executor: Cursor Agent/Composer (Standard, Fast OFF). Human/ChatGPT reviews architecture and evidence.
- Every execution request has a unique PROMPT-ID; include that ID in final report.
- Use a fresh feature branch and a **Draft PR**; do not merge, tag, release or deploy without explicit authorization.
- Report start SHA, end SHA, changed files, tests run, failures, safety risks, PR URL and blockers.
- Scope changes minimally; do not independently expand to unrelated features.

## Safety invariants
- No mutation of ZFS pools, partitions, devices or VM storage. Never invoke zpool create/destroy/replace/clear/online/offline, zfs set/destroy, disk format, or firmware commands.
- Pollers are read-only. Use fixed executable paths and argument arrays; never build shell commands from device names.
- Unknown, missing, partial, stale or malformed telemetry is **UNKNOWN**, never healthy.
- Never auto-install services, change ZED/SMART/Proxmox config, or send real email during tests.
- Network delivery is opt-in, disabled by default. Never commit real emails, tokens, internal IPs or serial numbers.
- Enforce bounded execution, outputs and cardinality; handle timeouts, NVMe missing/unavailable and non-ZFS hosts.
- Alerts must deduplicate and produce recovery only on verified transitions. Notifications must never trigger disk operations.
- Keep public PR CI on GitHub-hosted runners with read-only token and no secrets. Never run untrusted public PR code on private self-hosted signing runners.
- Do not introduce a Proxmox GUI plugin or expose a new listening port without a reviewed requirement.

## Definition of done per PR
`gofmt`, `go test ./...`, `go vet ./...`, CGO-disabled build, negative fixture tests, updated docs and a reproducible test report. Mark untested integrations as **not verified**.
