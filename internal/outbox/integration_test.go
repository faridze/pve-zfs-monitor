package outbox

import (
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/observium"
)

// End-to-end offline path: detection events -> outbox -> fake mail + agent section.
func TestMonitorEventsToOutboxAndObserviumExport(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	k, _ := monitor.Key("pool", "private-pool")
	sig := monitor.Signal{Key: k, Level: monitor.Critical, Verified: true, ObservedAt: now}
	_, events, err := monitor.Decide(monitor.State{}, []monitor.Signal{sig}, now, monitor.Options{})
	if err != nil || len(events) != 1 {
		t.Fatalf("decide: events=%+v err=%v", events, err)
	}
	store := &MemoryStore{}
	if err := store.Enqueue(events, now); err != nil {
		t.Fatal(err)
	}
	fake := &FakeTransport{}
	if _, err := store.DeliverPending("lab", fake, now); err != nil {
		t.Fatal(err)
	}
	sum, err := observium.SummarizeSignals([]monitor.Signal{sig}, now, 1)
	if err != nil {
		t.Fatal(err)
	}
	section, err := observium.RenderAppSection(sum)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observium.ParseAppSection(section); err != nil {
		t.Fatal(err)
	}
}
