package outbox

import "github.com/faridze/pve-zfs-monitor/internal/monitor"

// Status is delivery state for one queued notification.
type Status string

const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Entry is one durable notification intent.
type Entry struct {
	ID          string        `json:"id"`
	Event       monitor.Event `json:"event"`
	CreatedAt   int64         `json:"created_at_unix"`
	DeliveredAt int64         `json:"delivered_at_unix,omitempty"`
	Attempts    int           `json:"attempts,omitempty"`
	Status      Status        `json:"status"`
	LastError   string        `json:"last_error,omitempty"`
}

// Snapshot is the on-disk outbox document.
type Snapshot struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

const (
	snapshotVersion = 1
	maxEntries      = 256
)

func validateEntry(e Entry) error {
	if e.ID == "" || len(e.ID) > 64 {
		return errInvalidEntry
	}
	if e.Status != StatusPending && e.Status != StatusDelivered && e.Status != StatusFailed {
		return errInvalidEntry
	}
	if e.CreatedAt <= 0 {
		return errInvalidEntry
	}
	if e.Attempts < 0 || e.Attempts > 100 {
		return errInvalidEntry
	}
	if e.Event.Kind != monitor.Alert && e.Event.Kind != monitor.Reminder && e.Event.Kind != monitor.Recovery {
		return errInvalidEntry
	}
	switch e.Event.Level {
	case monitor.Healthy, monitor.Warning, monitor.Critical, monitor.Unknown:
	default:
		return errInvalidEntry
	}
	if len(e.Event.Key) < 10 || len(e.Event.Key) > 80 {
		return errInvalidEntry
	}
	return nil
}
