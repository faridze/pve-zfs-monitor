# P2-C: ZFS member topology and SMART/NVMe detection (read-only)

**Status:** internal library code only. No polling scheduler, SMART self-tests, ZFS scrub, device resets, mail, Observium, or production installation.

P2-C adds an offline normalization and detection layer on top of existing collectors (`internal/zfs`, `internal/device`) and the P2-A/P2-B monitor engine (`Detect`, `DecideWithDetection`). CLI behavior is unchanged.

## What is evaluated

- ZFS pool list health (existing `PoolSignals`) plus per-member topology from `zpool status` leaves: `ONLINE`/`INUSE`, `DEGRADED`, `FAULTED`/`OFFLINE`/`UNAVAIL`, and unknown states.
- ZFS `READ` / `WRITE` / `CKSUM` counters per member, including increases between verified observations.
- SMART overall pass/fail, NVMe `critical_warning`, media and data-integrity error counters, available spare vs vendor spare threshold.
- Optional, conservative thresholds for temperature and percentage used (`DetectionOptions`; **0 disables**).
- Incomplete SMART inventory, collection failures, ambiguous or missing member mappings, and duplicate paths claiming the same stable hardware identity.

## Privacy and evidence rules

- Persisted state stores only hashed `Key(scope, identity)` values and counter baselines. No serial numbers, WWNs, `/dev/*` paths, or raw SMART JSON in `state.json`.
- Topology members use `device:<hash(hw:stable_id)>` when mapped; SMART uses a separate `smart:` identity prefix so matching statistics on two disks never proves they are the same physical device.
- Missing values and zero counters do **not** prove health. Historical nonzero counters without a verified increase do **not** raise new alerts.
- Counter decreases or resets, identity changes, and insufficient mapping evidence yield `unknown` rather than a false positive.
- Model-specific thresholds remain opt-in until validated in the deployment environment.

## Persistence

`State.Baselines` holds last verified ZFS and SMART counters keyed by signal key. `DecideWithDetection` runs `Detect` then `Decide` and attaches baselines to the next state. Corrupt baselines fail state validation (same contract as P2-B).

## Limitations

- Durable state is not exactly-once notification delivery.
- No automatic enablement of temperature or wear thresholds in production defaults.
- Synthetic fixtures only in CI; a non-production PVE lab is still required before operational enablement.

## Synthetic acceptance

In a Linux development environment:

```bash
gofmt -w internal/monitor
go test ./...
go test -race ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/pve-zfs-monitor
```

Fixtures under `internal/zfs/testdata` and `internal/device/testdata` cover healthy mirrors, degraded members, rising and reset counters, SMART failure, missing SMART, temperature thresholds, ambiguous mapping, and verified recovery via `DecideWithDetection`.

## Change management

Targets `main` after merged P2-A and P2-B. Draft PR only — no merge, tag, release, or deploy without explicit approval.
