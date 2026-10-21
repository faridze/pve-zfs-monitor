package outbox

import (
	"strings"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

// Status is delivery state for one queued notification.
type Status string

const (
	StatusPending   Status = "pending"
	StatusDelivered Status = "delivered"
	StatusFailed    Status = "failed"
)

// Entry is one durable notification intent.
type Entry struct {
	ID            string        `json:"id"`
	LogicalID     string        `json:"logical_id"`
	Event         monitor.Event `json:"event"`
	CreatedAt     int64         `json:"created_at_unix"`
	DeliveredAt   int64         `json:"delivered_at_unix,omitempty"`
	NextRetryUnix int64         `json:"next_retry_unix,omitempty"`
	Attempts      int           `json:"attempts,omitempty"`
	Status        Status        `json:"status"`
	LastError     string        `json:"last_error,omitempty"`
}

// Snapshot is the on-disk outbox document.
type Snapshot struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

const (
	SnapshotVersion = 1
	snapshotVersion = SnapshotVersion
	maxEntries      = 256
)

func validateEntry(e Entry) error {
	if !validEntryID(e.ID) {
		return errInvalidEntry
	}
	if !validLogicalID(e.LogicalID) {
		return errInvalidEntry
	}
	fp, err := entryLogicalID(e.Event)
	if err != nil || fp != e.LogicalID {
		return errInvalidEntry
	}
	if e.Status != StatusPending && e.Status != StatusDelivered && e.Status != StatusFailed {
		return errInvalidEntry
	}
	if e.CreatedAt <= 0 {
		return errInvalidEntry
	}
	if e.NextRetryUnix < 0 {
		return errInvalidEntry
	}
	if e.Attempts < 0 || e.Attempts > maxAttempts {
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
	if !monitor.ValidSignalKey(e.Event.Key) {
		return errInvalidEntry
	}
	if e.Event.EmittedUnix <= 0 {
		return errInvalidEntry
	}
	if strings.Contains(e.LastError, "@") {
		return errInvalidEntry
	}
	switch e.Status {
	case StatusPending:
		if e.DeliveredAt != 0 || e.Attempts != 0 {
			return errInvalidEntry
		}
	case StatusDelivered:
		if e.DeliveredAt <= 0 || e.Attempts < 1 {
			return errInvalidEntry
		}
	case StatusFailed:
		if e.Attempts < 1 {
			return errInvalidEntry
		}
		if e.Attempts < maxAttempts && e.NextRetryUnix <= 0 {
			return errInvalidEntry
		}
		if e.Attempts >= maxAttempts && e.NextRetryUnix != 0 {
			return errInvalidEntry
		}
	}
	return nil
}
