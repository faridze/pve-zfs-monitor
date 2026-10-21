package outbox

import (
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func hostKey() string {
	k, _ := monitor.Key("host", "lifecycle-test")
	return k
}

func TestIncidentLifecycleCriticalRecoveryCritical(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	key := hostKey()
	q := &Queue{}
	alert1 := monitor.Event{Key: key, Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: 100}
	if err := q.EnqueueDeduped([]monitor.Event{alert1}, now); err != nil {
		t.Fatal(err)
	}
	id1 := q.entries[0].ID
	fake := &FakeTransport{}
	if _, err := DeliverQueue(q, "lab", fake, now); err != nil || len(fake.Sent) != 1 {
		t.Fatalf("deliver alert: sent=%d", len(fake.Sent))
	}
	recovery := monitor.Event{Key: key, Kind: monitor.Recovery, Level: monitor.Healthy, EmittedUnix: 200}
	if err := q.EnqueueDeduped([]monitor.Event{recovery}, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := DeliverQueue(q, "lab", fake, now.Add(time.Minute)); err != nil || len(fake.Sent) != 2 {
		t.Fatalf("deliver recovery: sent=%d", len(fake.Sent))
	}
	alert2 := monitor.Event{Key: key, Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: 300}
	if err := q.EnqueueDeduped([]monitor.Event{alert2}, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(q.entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(q.entries))
	}
	if q.entries[2].ID == id1 {
		t.Fatal("new alert must have new entry id")
	}
	if err := q.EnqueueDeduped([]monitor.Event{alert2}, now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if len(q.entries) != 3 {
		t.Fatalf("duplicate unchanged incident must not add entry, got %d", len(q.entries))
	}
}

func TestDeliveredCompactionFreesCapacity(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	q := &Queue{}
	key := hostKey()
	oldDelivered := now.Add(-72 * time.Hour)
	for i := 0; i < 50; i++ {
		ev := monitor.Event{Key: key, Kind: monitor.Alert, Level: monitor.Warning, EmittedUnix: int64(100 + i)}
		if err := q.EnqueueDeduped([]monitor.Event{ev}, now.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
		idx := len(q.entries) - 1
		ApplyDeliveryResult(&q.entries[idx], now.Add(time.Duration(i)*time.Second), nil)
	}
	for i := range q.entries {
		q.entries[i].DeliveredAt = oldDelivered.Unix()
	}
	removed := q.CompactDelivered(now)
	if removed != 50 {
		t.Fatalf("expected 50 removed, got %d", removed)
	}
	ev := monitor.Event{Key: key, Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: 9999}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatalf("enqueue after compaction: %v", err)
	}
}

func TestSanitizeTransportErrorRedactsEmail(t *testing.T) {
	var e Entry
	ApplyDeliveryResult(&e, time.Unix(1, 0), &emailError{})
	if e.LastError != "delivery failed" {
		t.Fatalf("redact: %q", e.LastError)
	}
}

type emailError struct{}

func (e *emailError) Error() string { return "550 rejected user@example.com" }
