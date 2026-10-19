# P2-B: opt-in durable alert state (Linux only)

**Status:** internal experimental code only. No polling scheduler, service, mail sender, Observium interface, or production installation.

The private, read-only P2-A decision engine needs state storage to remember alerts across runs. This PR adds only an *opt-in*, atomic local filesystem implementation called `FileStore.Advance`. Existing CLI behavior is unchanged.

## Persistence and safety contract

1. Require an existing, clean absolute directory. Every component is opened with `openat(O_NOFOLLOW|O_DIRECTORY)`; symlinks and relative paths are rejected. The leaf directory must be private (0700), owned by the caller.
2. Open fixed `state.lock` with `O_NOFOLLOW`, reject nonregular, insecure or hardlinked lock files, and take an exclusive nonblocking flock for read/decide/write.
3. Load fixed `state.json` with private 0600 permissions, correct owner, single hardlink and a 128 KiB maximum. **Absent file** means first run. **Present file** must decode to schema version `1` with a non-null `records` map and valid record metadata; `{}`, `null`, `version: 0`, unsupported versions, unknown JSON fields, truncated JSON, invalid levels/timestamps/streaks and oversize payloads are errors. Corrupt existing data is **never** interpreted as healthy or reset silently.
4. Evaluate pure `Decide` under the same lock. For a valid next state, create a random private same-directory file using `O_EXCL`; fsync file, rename into place, then fsync directory. Only return event decisions after successful persistence.
5. No storage initialization or local host configuration is carried out automatically. The library has no network, email, ZFS, SMART, or mail APIs.

Only hashed scoped identifiers and minimal dedupe metadata are persisted. **SHA-256 digests are not anonymity guarantees** for guessable low-entropy names; keep state private.

## Limitations that remain

- This is durable state, **not exactly-once notification delivery**. A future delivery adapter requires a transactional outbox and acknowledgments so process interruption cannot lose events.
- Bad state deliberately blocks alerts until an operator has inspected it; no silent correction, deletion, or reset is attempted.
- No automatic threshold policy for devices, ZFS scrub, or mirror member diagnostics.
- No test or install on a production node. A distinct non-production PVE VM will be needed before operational enablement.

## Synthetic acceptance

`go test -count=1 ./internal/monitor` and `go test -race ./internal/monitor` in a Linux development environment.

Tests exercise persistent dedupe, reminder and verified recovery, private file modes, invalid and oversized state, symlinked ancestors/state/lock, wrong permissions, concurrent locking and no changes for malformed readings.

## Change management

**P2-A (alert decision engine)** is merged on `main`. This Draft PR targets `main` directly. Neither PR is implicitly approved for Merge, Tag, Release, or Deploy.
