package outbox

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

const (
	maxAttempts      = 10
	baseRetrySeconds = 30
	maxRetrySeconds  = 3600
)

// Queue holds notification entries in memory or as part of a snapshot.
type Queue struct {
	entries []Entry
}

func (q *Queue) Entries() []Entry {
	return append([]Entry(nil), q.entries...)
}

// EnqueueDeduped appends events unless an equivalent pending/failed-retryable entry exists.
func (q *Queue) EnqueueDeduped(events []monitor.Event, now time.Time) error {
	if now.IsZero() {
		return errors.New("invalid time")
	}
	for _, ev := range events {
		fp := eventFingerprint(ev)
		if q.hasActiveFingerprint(fp) {
			continue
		}
		id, err := randomID()
		if err != nil {
			return err
		}
		entry := Entry{
			ID:        id,
			Event:     ev,
			CreatedAt: now.Unix(),
			Status:    StatusPending,
			LogicalID: fp,
		}
		if err := validateEntry(entry); err != nil {
			return err
		}
		if len(q.entries) >= maxEntries {
			return errFull
		}
		q.entries = append(q.entries, entry)
	}
	return nil
}

func (q *Queue) hasActiveFingerprint(fp string) bool {
	for _, e := range q.entries {
		if e.LogicalID != fp {
			continue
		}
		if e.Status == StatusPending || e.Status == StatusDelivered {
			return true
		}
		if e.Status == StatusFailed && e.Attempts < maxAttempts {
			return true
		}
	}
	return false
}

// Deliverable returns entries ready for delivery at now, ordered by severity then age.
func (q *Queue) Deliverable(now time.Time) []int {
	if now.IsZero() {
		return nil
	}
	var idxs []int
	for i, e := range q.entries {
		if e.Status == StatusDelivered {
			continue
		}
		if e.Status == StatusPending || (e.Status == StatusFailed && e.Attempts < maxAttempts && now.Unix() >= e.NextRetryUnix) {
			idxs = append(idxs, i)
		}
	}
	sort.Slice(idxs, func(a, b int) bool {
		ea, eb := q.entries[idxs[a]], q.entries[idxs[b]]
		pa, pb := deliveryPriority(ea.Event), deliveryPriority(eb.Event)
		if pa != pb {
			return pa < pb
		}
		return ea.CreatedAt < eb.CreatedAt
	})
	return idxs
}

func deliveryPriority(e monitor.Event) int {
	switch e.Kind {
	case monitor.Alert:
		switch e.Level {
		case monitor.Critical:
			return 0
		case monitor.Warning:
			return 1
		default:
			return 2
		}
	case monitor.Reminder:
		return 3
	case monitor.Recovery:
		return 4
	default:
		return 5
	}
}

// ApplyDeliveryResult updates an entry after transport attempt.
func ApplyDeliveryResult(e *Entry, now time.Time, sendErr error) {
	e.Attempts++
	if sendErr == nil {
		e.Status = StatusDelivered
		e.DeliveredAt = now.Unix()
		e.LastError = ""
		e.NextRetryUnix = 0
		return
	}
	e.Status = StatusFailed
	e.LastError = truncateErr(sendErr.Error())
	if e.Attempts >= maxAttempts {
		e.NextRetryUnix = 0
		return
	}
	delay := baseRetrySeconds
	for i := 1; i < e.Attempts; i++ {
		delay *= 2
		if delay > maxRetrySeconds {
			delay = maxRetrySeconds
			break
		}
	}
	e.NextRetryUnix = now.Unix() + int64(delay)
}

func eventFingerprint(e monitor.Event) string {
	sum := sha256.Sum256([]byte(string(e.Kind) + "\x00" + e.Key + "\x00" + string(e.Level)))
	return hex.EncodeToString(sum[:16])
}

func truncateErr(s string) string {
	if len(s) > 200 {
		return s[:200]
	}
	return s
}

func (q *Queue) LoadSnapshot(s Snapshot) error {
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
	q.entries = out
	return nil
}

func (q *Queue) Snapshot() Snapshot {
	entries := append([]Entry(nil), q.entries...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].CreatedAt < entries[j].CreatedAt })
	return Snapshot{Version: snapshotVersion, Entries: entries}
}

func (q *Queue) PendingCount() int {
	n := 0
	for _, e := range q.entries {
		if e.Status == StatusPending || (e.Status == StatusFailed && e.Attempts < maxAttempts) {
			n++
		}
	}
	return n
}
