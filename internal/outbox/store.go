package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

// MemoryStore is an in-memory outbox for offline tests (no filesystem).
type MemoryStore struct {
	entries []Entry
}

// Enqueue appends new events as pending entries with unique IDs.
func (m *MemoryStore) Enqueue(events []monitor.Event, now time.Time) error {
	if now.IsZero() {
		return errors.New("invalid time")
	}
	if len(events) == 0 {
		return nil
	}
	if len(m.entries)+len(events) > maxEntries {
		return errFull
	}
	seen := map[string]bool{}
	for _, e := range m.entries {
		seen[e.ID] = true
	}
	for _, ev := range events {
		id, err := randomID()
		if err != nil {
			return err
		}
		entry := Entry{
			ID:        id,
			Event:     ev,
			CreatedAt: now.Unix(),
			Status:    StatusPending,
		}
		if err := validateEntry(entry); err != nil {
			return err
		}
		m.entries = append(m.entries, entry)
	}
	return nil
}

// Pending returns pending entries in creation order.
func (m *MemoryStore) Pending() []Entry {
	out := make([]Entry, 0)
	for _, e := range m.entries {
		if e.Status == StatusPending {
			out = append(out, e)
		}
	}
	return out
}

// DeliverPending sends pending entries through transport and marks delivered/failed.
func (m *MemoryStore) DeliverPending(hostLabel string, transport Transport, now time.Time) (int, error) {
	if transport == nil {
		return 0, errors.New("missing transport")
	}
	if now.IsZero() {
		return 0, errors.New("invalid time")
	}
	delivered := 0
	for i := range m.entries {
		if m.entries[i].Status != StatusPending {
			continue
		}
		msg, err := FormatProxmoxStyle(hostLabel, m.entries[i].Event)
		if err != nil {
			m.entries[i].Status = StatusFailed
			m.entries[i].LastError = err.Error()
			m.entries[i].Attempts++
			continue
		}
		if err := transport.Send(nil, msg); err != nil {
			m.entries[i].Status = StatusFailed
			m.entries[i].LastError = err.Error()
			m.entries[i].Attempts++
			continue
		}
		m.entries[i].Status = StatusDelivered
		m.entries[i].DeliveredAt = now.Unix()
		m.entries[i].Attempts++
		delivered++
	}
	return delivered, nil
}

// Snapshot exports the current queue for persistence roundtrips.
func (m *MemoryStore) Snapshot() Snapshot {
	entries := append([]Entry(nil), m.entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt < entries[j].CreatedAt })
	return Snapshot{Version: snapshotVersion, Entries: entries}
}

// LoadSnapshot replaces contents from a validated snapshot.
func (m *MemoryStore) LoadSnapshot(s Snapshot) error {
	if s.Version != snapshotVersion {
		return errInvalidSnapshot
	}
	if len(s.Entries) > maxEntries {
		return errInvalidSnapshot
	}
	out := make([]Entry, 0, len(s.Entries))
	for _, e := range s.Entries {
		if err := validateEntry(e); err != nil {
			return err
		}
		out = append(out, e)
	}
	m.entries = out
	return nil
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

func randomID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("random id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
