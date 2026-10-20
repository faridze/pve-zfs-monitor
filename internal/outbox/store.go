package outbox

import (
	"encoding/json"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

// MemoryStore is an in-memory outbox for offline tests (no filesystem).
type MemoryStore struct {
	q Queue
}

func (m *MemoryStore) Enqueue(events []monitor.Event, now time.Time) error {
	return m.q.EnqueueDeduped(events, now)
}

func (m *MemoryStore) Pending() []Entry {
	var out []Entry
	for _, e := range m.q.entries {
		if e.Status == StatusPending {
			out = append(out, e)
		}
	}
	return out
}

func (m *MemoryStore) PendingCount() int {
	return m.q.PendingCount()
}

// DeliverPending sends deliverable entries (pending and failed-retryable).
func (m *MemoryStore) DeliverPending(hostLabel string, transport Transport, now time.Time) (int, error) {
	return DeliverQueue(&m.q, hostLabel, transport, now)
}

func (m *MemoryStore) Snapshot() Snapshot {
	return m.q.Snapshot()
}

func (m *MemoryStore) LoadSnapshot(s Snapshot) error {
	return m.q.LoadSnapshot(s)
}

func (m *MemoryStore) MarshalJSON() ([]byte, error) {
	return json.Marshal(m.Snapshot())
}

func (m *MemoryStore) UnmarshalJSON(data []byte) error {
	var s Snapshot
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	return m.LoadSnapshot(s)
}

// DeliverQueue delivers ready entries and persists results in q.
func DeliverQueue(q *Queue, hostLabel string, transport Transport, now time.Time) (int, error) {
	if transport == nil {
		return 0, errInvalidSnapshot
	}
	if now.IsZero() {
		return 0, errInvalidSnapshot
	}
	delivered := 0
	for _, i := range q.Deliverable(now) {
		msg, err := FormatMessage(hostLabel, q.entries[i].Event)
		if err != nil {
			ApplyDeliveryResult(&q.entries[i], now, err)
			continue
		}
		err = transport.Send(nil, msg)
		ApplyDeliveryResult(&q.entries[i], now, err)
		if err == nil {
			delivered++
		}
	}
	return delivered, nil
}
