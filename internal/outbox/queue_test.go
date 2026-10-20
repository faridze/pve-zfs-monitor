package outbox

import (
	"errors"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestFailedEntryRetriesAfterBackoff(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ev := monitor.Event{Key: "host:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Kind: monitor.Alert, Level: monitor.Critical}
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	failing := &FakeTransport{Err: errors.New("smtp down")}
	if _, err := DeliverQueue(q, "lab", failing, now); err != nil {
		t.Fatal(err)
	}
	if q.Deliverable(now.Add(time.Minute)) != nil {
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
	ev := monitor.Event{Key: "host:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Kind: monitor.Alert, Level: monitor.Critical}
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
	crit := monitor.Event{Key: "host:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Kind: monitor.Alert, Level: monitor.Critical}
	rec := monitor.Event{Key: "host:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Kind: monitor.Recovery, Level: monitor.Healthy}
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
	ev := monitor.Event{Key: "host:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", Kind: monitor.Alert, Level: monitor.Warning}
	q := &Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev, ev}, now); err != nil {
		t.Fatal(err)
	}
	if len(q.entries) != 1 {
		t.Fatalf("dedupe expected 1 entry, got %d", len(q.entries))
	}
}
