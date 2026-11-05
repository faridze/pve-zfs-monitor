# P4 lab acceptance checklist

Use on a **disposable Proxmox VE 8/9 VM** with VM snapshot before/after. **Fail** if any safety rule is violated.

## Safety gates (must all pass)

- [ ] VM is not production; hostname documented as lab-only
- [ ] No ZFS pool create/destroy/replace/clear during monitor tests
- [ ] Lab state directory separate from any production path (`0700`)
- [ ] No systemd timer or cron enabled unless this checklist section signed off
- [ ] No modification to ZED, smartd, Postfix, Proxmox matchers, Observium prod
- [ ] No real customer email addresses or SMTP credentials used
- [ ] Port 36602 not listening unless authorization row below signed

**Authorization (optional):** TCP 36602 enabled ___date___ operator ___name___ duration ___

## Install / rollback

- [ ] Installed lab-signed or dev binary per [install.md](../install.md) (channel documented)
- [ ] `pve-zfs-monitor version --json` shows expected build
- [ ] `rollback` restores previous binary (if applicable)
- [ ] VM snapshot restored OR lab state dir removed after session

## Collection CLI (read-only)

- [ ] `status --json` / `check` exit codes documented for lab pool layout
- [ ] `topology --json` and `devices --json` complete without secrets in output
- [ ] Commands respect timeouts; no hung processes

## Observium track

Reference: [observium-validation.md](observium-validation.md)

- [ ] O1–O8 CI tests green on tested commit SHA
- [ ] L-O1: Agent section generated on lab node
- [ ] L-O2: Degraded case shows `report_fully_healthy=0`
- [ ] L-O3: Incomplete SMART not fully healthy
- [ ] L-O4: Stale flag when observations aged
- [ ] L-O5–L-O8 (if Observium lab used): app discovered and polled without false green
- [ ] No pool names / serials / paths in agent section (grep)

## Mail / outbox track

Reference: [mail-validation.md](mail-validation.md)

- [ ] M1–M6 CI tests green
- [ ] L-M1–L-M4 lab log-sink or FakeTransport harness
- [ ] Concurrent dispatch: no duplicate send (second dispatcher busy)
- [ ] Crash-after-send test documents duplicate if injected (D5)
- [ ] System mail **not** enabled unless Phase 3 implementation exists

## Durability track

Reference: [durability-test-matrix.md](durability-test-matrix.md)

- [ ] D9 corrupt file not overwritten (sha256 match)
- [ ] D12 legacy migration succeeds
- [ ] D7 concurrent dispatch CI/lab pass
- [ ] Lab-only: D15/D16 permission/disk-full (optional advanced)

## Alert policy

Reference: [alert-routing-policy.md](alert-routing-policy.md)

- [ ] Authoritative notification channel chosen and documented
- [ ] Overlap with ZED/Observium understood; duplicate count acceptable for lab

## Final sign-off

| Field | Value |
|-------|--------|
| Commit SHA tested | |
| Lab VM ID | |
| Observium version (if used) | |
| PVE version | |
| Result | PASS / FAIL |
| Reviewer | |
| Date | |

**PASS** requires all safety gates and selected track items for the agreed P4 scope. **FAIL** blocks merge of any future “lab verified” claims in README or release notes.

## Next execution step after this checklist

Implement and review **lab orchestration CLI** (collect → detect → advance → optional deliver with FakeTransport/log sink) and re-run this checklist against that entrypoint.
