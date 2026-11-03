# Durability and fault-injection test matrix

Maps P3 `internal/delivery`, `internal/outbox`, and `internal/persistjson` behavior to lab and CI verification. **Expected:** durable notification **intent**; **at-least-once** external delivery; no silent loss of pending Critical without explicit terminal failure state.

## Legend

| Outcome | Meaning |
|---------|---------|
| **Fail-closed** | Load/advance/deliver error; file unchanged |
| **Durable** | Intent survives restart |
| **At-least-once** | Duplicate external delivery possible after crash-after-send |
| **CI** | Covered in GitHub/Cursor `go test` today |
| **Lab** | Requires Linux VM fault injection |

| ID | Scenario | Injection method | Expected behavior | Coverage |
|----|----------|------------------|-------------------|----------|
| D1 | First run (no state file) | Empty state dir | Empty doc; advance creates v2 state | CI: `document_test`, integration |
| D2 | Crash before state persistence | Kill during `AdvanceDetection` (lab: SIGKILL loop) | Prior state unchanged; new events not partially applied | Lab |
| D3 | Crash after atomic persistence | Kill after `atomicWriteAt` returns | Full state+outbox visible on restart | Lab + CI restart tests |
| D4 | Crash before transport | Pending entries remain | Deliver on next run | CI: restart before delivery |
| D5 | Crash after send, before ACK | Send succeeds; kill before second `state.lock` write | Retry delivers again (**at-least-once**) | CI: `TestAtLeastOnceWhenAckNotPersisted` |
| D6 | Restart + retry backoff | Clock synthetic / wait `NextRetryUnix` | Failed → delivered | CI: `queue_test`, scenarios |
| D7 | Concurrent dispatchers | Two `Coordinator.DeliverNotifications` | Second: `ErrDispatchBusy`; one send | CI: `concurrent_linux_test` |
| D8 | Full outbox (256 entries) | Fill with delivered + pending | `errFull`; Critical not silently dropped | CI: lifecycle compaction |
| D9 | Corrupt/truncated JSON | Write truncated `state.json` | Load/advance/deliver fail-closed; **file bytes unchanged** | CI: `document_integrity_linux_test` |
| D10 | Duplicate JSON keys | `{"state":...,"state":...}` | Reject on load | CI: `persistjson/strict_test` |
| D11 | Outbox typo `entires` | Legacy/v2 corrupt outbox field | Reject | CI: strict + integrity tests |
| D12 | Legacy v1 migration | `{"version":1,"records":{}}` | Migrate to v2 + empty outbox | CI: integration |
| D13 | Null outbox entries | `"entries":null` | Reject | CI: document_test |
| D14 | Missing entries (version only) | `"outbox":{"version":1}` | Migrate to `entries:[]` | CI: document_test |
| D15 | Permission failure | `chmod 000` state dir (lab) | Operations error; no partial write | Lab |
| D16 | Disk full | `ENOSPC` on state dir (lab loopback) | Advance/deliver error; no truncate | Lab |
| D17 | Recovery after verified healthy | Two-sample recovery in monitor | Recovery event delivered once per incident | CI: scenarios |
| D18 | False recovery | Single healthy sample | No recovery event | CI: monitor tests |
| D19 | Terminal failed Critical | Exhaust `maxAttempts` | Entry remains visible; not silently deleted | CI: queue policy + docs |
| D20 | Legacy + pending preservation | v1 state with simulated pending (future file) | Must not discard on migration | Lab when CLI wired |

## Lab execution script outline (operator)

```bash
# Example — NOT automated in repo; run on disposable VM only
LAB_STATE=/var/lib/pve-zfs-monitor-lab
install -d -m 0700 "$LAB_STATE"
# 1) Baseline advance + pending count
# 2) kill -9 during deliver (D5)
# 3) cmp state.json sha256 before/after corrupt injection (D9)
# 4) restore from VM snapshot on completion
```

## Explicit non-goals

- Proving exactly-once email or Observium alerts.
- Testing ZFS pool destruction or member replace.
- Running fault injection on production state paths.

## Sign-off

All **CI** rows must remain green on `main`. **Lab** rows D2, D3, D15, D16 require checklist approval in [lab-acceptance-checklist.md](lab-acceptance-checklist.md).
