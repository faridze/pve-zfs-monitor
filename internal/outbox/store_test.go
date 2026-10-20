package outbox

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func TestOutboxEnqueueDeliverFakeTransport(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	ev := monitor.Event{
		Key:   "pool:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Kind:  monitor.Alert,
		Level: monitor.Critical,
	}
	store := &MemoryStore{}
	if err := store.Enqueue([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	fake := &FakeTransport{}
	n, err := store.DeliverPending("lab-node", fake, now)
	if err != nil || n != 1 {
		t.Fatalf("deliver=%d err=%v", n, err)
	}
	if len(fake.Sent) != 1 {
		t.Fatalf("sent=%d", len(fake.Sent))
	}
	if fake.Sent[0].Subject == "" || fake.Sent[0].Body == "" {
		t.Fatalf("empty message: %+v", fake.Sent[0])
	}
	if strings.Contains(fake.Sent[0].Body, "rpool") || strings.Contains(fake.Sent[0].Body, "/dev/") {
		t.Fatalf("body leaked path: %s", fake.Sent[0].Body)
	}
	pending := store.Pending()
	if len(pending) != 0 {
		t.Fatalf("still pending: %+v", pending)
	}
}

func TestOutboxJSONRoundTrip(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	store := &MemoryStore{}
	_ = store.Enqueue([]monitor.Event{{
		Key:   "host:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Kind:  monitor.Reminder,
		Level: monitor.Warning,
	}}, now)
	data, err := json.Marshal(store)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &MemoryStore{}
	if err := json.Unmarshal(data, loaded); err != nil {
		t.Fatal(err)
	}
	if len(loaded.Pending()) != 1 {
		t.Fatalf("expected pending after load")
	}
}
