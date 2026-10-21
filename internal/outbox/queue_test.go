package outbox

import (
	"errors"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func testKey() string {
	k, _ := monitor.Key("host", "queue-test")
	return k
}

func testEvent(kind monitor.Kind, level monitor.Level, emitted int64) monitor.Event {
	return monitor.Event{Key: testKey(), Kind: kind, Level: level, EmittedUnix: emitted}
}

func TestFailedEntryRetriesAfterBackoff(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ev := testEvent(monitor.Alert, monitor.Critical, 100)
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	failing := &FakeTransport{Err: errors.New("smtp down")}
	if _, err := DeliverQueue(q, "lab", failing, now); err != nil {
		t.Fatal(err)
	}
	if len(q.Deliverable(now.Add(time.Minute))) != 0 {
		// still in backoff
	}
	idxs := q.Deliverable(now.Add(31 * time.Second))
	if len(idxs) == 0 {
		t.Fatal("expected retry after backoff")
	}
	fake := &FakeTransport{}
	n, err := DeliverQueue(q, "lab", fake, now.Add(31*time.Second))
	if err != nil || n != 1 {
		t.Fatalf("retry deliver n=%d err=%v", n, err)
	}
}

func TestAtLeastOnceWhenAckNotPersisted(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	ev := testEvent(monitor.Alert, monitor.Critical, 100)
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	fake := &FakeTransport{}
	msg, err := FormatMessage("lab", ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := fake.Send(nil, msg); err != nil {
		t.Fatal(err)
	}
	// Simulate crash after transport success but before durable ack.
	n, err := DeliverQueue(q, "lab", fake, now)
	if err != nil || n != 1 || len(fake.Sent) != 2 {
		t.Fatalf("expected duplicate delivery on retry n=%d sent=%d err=%v", n, len(fake.Sent), err)
	}
}

func TestCriticalDeliveredBeforeRecovery(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	k1, _ := monitor.Key("host", "prio-a")
	k2, _ := monitor.Key("host", "prio-b")
	crit := monitor.Event{Key: k1, Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: 1}
	rec := monitor.Event{Key: k2, Kind: monitor.Recovery, Level: monitor.Healthy, EmittedUnix: 2}
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{rec, crit}, now); err != nil {
		t.Fatal(err)
	}
	idxs := q.Deliverable(now)
	if len(idxs) != 2 || q.entries[idxs[0]].Event.Kind != monitor.Alert {
		t.Fatalf("ordering: %+v", idxs)
	}
}

func TestDedupeIdenticalPendingEvents(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	ev := testEvent(monitor.Alert, monitor.Warning, 50)
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev, ev}, now); err != nil {
		t.Fatal(err)
	}
	if len(q.entries) != 1 {
		t.Fatalf("dedupe expected 1 entry, got %d", len(q.entries))
	}
}
