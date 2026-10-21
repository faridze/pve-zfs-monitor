package outbox

import (
	"context"
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

// DeliverQueue delivers ready entries and persists results in q (in-process lock holder).
func DeliverQueue(q *Queue, hostLabel string, transport Transport, now time.Time) (int, error) {
	if transport == nil {
		return 0, errInvalidSnapshot
	}
	if now.IsZero() {
		return 0, errInvalidSnapshot
	}
	var results []DeliveryResult
	for _, i := range q.Deliverable(now) {
		entry := q.entries[i]
		msg, err := FormatMessage(hostLabel, entry.Event)
		if err != nil {
			results = append(results, DeliveryResult{ID: entry.ID, SendErr: err})
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), TransportTimeout)
		sendErr := transport.Send(ctx, msg)
		cancel()
		results = append(results, DeliveryResult{ID: entry.ID, SendErr: sendErr})
	}
	return q.ApplyDeliveryResults(results, now), nil
}

// EntryIDsForDelivery returns stable IDs and messages for deliverable entries.
func (q *Queue) EntryIDsForDelivery(hostLabel string, now time.Time) (ids []string, msgs []Message, formatErrs map[string]error) {
	formatErrs = map[string]error{}
	for _, i := range q.Deliverable(now) {
		entry := q.entries[i]
		msg, err := FormatMessage(hostLabel, entry.Event)
		if err != nil {
			formatErrs[entry.ID] = err
			continue
		}
		ids = append(ids, entry.ID)
		msgs = append(msgs, msg)
	}
	return ids, msgs, formatErrs
}
