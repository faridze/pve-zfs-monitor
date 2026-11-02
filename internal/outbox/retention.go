package outbox

import "time"

// DeliveredRetention is how long delivered entries are kept before compaction.
const DeliveredRetention = 48 * time.Hour

// CompactDelivered removes old delivered entries. Never removes pending, retryable
// failures, or terminal failures for undelivered Critical events.
func (q *Queue) CompactDelivered(now time.Time) int {
	if now.IsZero() {
		return 0
	}
	cutoff := now.Add(-DeliveredRetention).Unix()
	kept := make([]Entry, 0, len(q.entries))
	removed := 0
	for _, e := range q.entries {
		if e.Status == StatusDelivered && e.DeliveredAt > 0 && e.DeliveredAt < cutoff {
			// Never drop if a later undelivered critical depends on ordering — delivered only.
			removed++
			continue
		}
		kept = append(kept, e)
	}
	q.entries = kept
	return removed
}

// prepareForEnqueue compacts delivered history then rejects if still at capacity
// with undeliverable backlog.
func (q *Queue) prepareForEnqueue(now time.Time) error {
	q.CompactDelivered(now)
	if len(q.entries) < maxEntries {
		return nil
	}
	return errFull
}
