package delivery

import (
	"context"
	"errors"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

// Coordinator atomically persists monitor transitions and notification intents.
type Coordinator struct {
	Store Store
}

// AdvanceDetection runs DecideWithDetection and enqueues emitted events in one durable write.
func (c Coordinator) AdvanceDetection(in monitor.DetectionInput, now time.Time, opts monitor.Options, detect monitor.DetectionOptions) (monitor.State, []monitor.Event, error) {
	if now.IsZero() {
		return monitor.State{}, nil, errors.New("invalid time")
	}
	var resultState monitor.State
	var resultEvents []monitor.Event
	err := c.Store.withLock(func(dir int) error {
		data, err := readFileAt(dir, stateFileName)
		if err != nil {
			return err
		}
		doc, err := decodeDocument(data)
		if err != nil {
			return err
		}
		next, events, err := monitor.DecideWithDetection(doc.State, in, now, opts, detect)
		if err != nil {
			return err
		}
		q := &outbox.Queue{}
		if err := q.LoadSnapshot(doc.Outbox); err != nil {
			return err
		}
		q.CompactDelivered(now)
		if err := q.EnqueueDeduped(events, now); err != nil {
			return err
		}
		doc.State = next
		doc.Outbox = q.Snapshot()
		encoded, err := encodeDocument(doc)
		if err != nil {
			return err
		}
		if len(encoded) > maxDocSize {
			return errors.New("document exceeds size limit")
		}
		if err := atomicWriteAt(dir, stateFileName, encoded); err != nil {
			return err
		}
		resultState = next
		resultEvents = events
		return nil
	})
	return resultState, resultEvents, err
}

// DeliverNotifications delivers pending/retryable outbox entries and persists acknowledgments.
// Transport runs outside the state lock; duplicate external delivery is possible if the process
// crashes after transport success but before the durable acknowledgment is written.
func (c Coordinator) DeliverNotifications(hostLabel string, transport outbox.Transport, now time.Time) (int, error) {
	if transport == nil {
		return 0, errors.New("missing transport")
	}
	var ids []string
	var msgs []outbox.Message
	var formatErrs map[string]error
	err := c.Store.withLock(func(dir int) error {
		data, err := readFileAt(dir, stateFileName)
		if err != nil {
			return err
		}
		doc, err := decodeDocument(data)
		if err != nil {
			return err
		}
		q := &outbox.Queue{}
		if err := q.LoadSnapshot(doc.Outbox); err != nil {
			return err
		}
		q.CompactDelivered(now)
		ids, msgs, formatErrs = q.EntryIDsForDelivery(hostLabel, now)
		return nil
	})
	if err != nil {
		return 0, err
	}
	var results []outbox.DeliveryResult
	for i, id := range ids {
		ctx, cancel := context.WithTimeout(context.Background(), outbox.TransportTimeout)
		sendErr := transport.Send(ctx, msgs[i])
		cancel()
		results = append(results, outbox.DeliveryResult{ID: id, SendErr: sendErr})
	}
	for id, ferr := range formatErrs {
		results = append(results, outbox.DeliveryResult{ID: id, SendErr: ferr})
	}
	delivered := 0
	err = c.Store.withLock(func(dir int) error {
		data, err := readFileAt(dir, stateFileName)
		if err != nil {
			return err
		}
		doc, err := decodeDocument(data)
		if err != nil {
			return err
		}
		q := &outbox.Queue{}
		if err := q.LoadSnapshot(doc.Outbox); err != nil {
			return err
		}
		delivered = q.ApplyDeliveryResults(results, now)
		q.CompactDelivered(now)
		doc.Outbox = q.Snapshot()
		encoded, err := encodeDocument(doc)
		if err != nil {
			return err
		}
		return atomicWriteAt(dir, stateFileName, encoded)
	})
	return delivered, err
}

// PendingNotifications returns the count of pending/retryable intents.
func (c Coordinator) PendingNotifications() (int, error) {
	doc, err := c.Store.Load()
	if err != nil {
		return 0, err
	}
	q := &outbox.Queue{}
	if err := q.LoadSnapshot(doc.Outbox); err != nil {
		return 0, err
	}
	return q.PendingCount(), nil
}
