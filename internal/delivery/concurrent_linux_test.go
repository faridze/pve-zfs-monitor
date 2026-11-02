//go:build linux

package delivery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

// gateTransport blocks in Send until Proceed is closed; signals EnterSend once per Send.
type gateTransport struct {
	EnterSend chan struct{}
	Proceed   chan struct{}
	sent      atomic.Int32
}

func (g *gateTransport) Send(_ context.Context, _ outbox.Message) error {
	g.sent.Add(1)
	select {
	case g.EnterSend <- struct{}{}:
	default:
	}
	<-g.Proceed
	return nil
}

func TestConcurrentDispatchersSingleSend(t *testing.T) {
	dir := privateDir(t)
	store := Store{Directory: dir}
	c1 := Coordinator{Store: store}
	c2 := Coordinator{Store: store}
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	key, _ := monitor.Key("host", "dispatch-serial-test")
	ev := monitor.Event{Key: key, Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: now.Unix()}
	q := &outbox.Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	doc := Document{
		FormatVersion: formatVersion2,
		State:         monitor.State{Version: monitor.StateVersion(), Records: map[string]monitor.Record{}},
		Outbox:        q.Snapshot(),
	}
	if err := store.Save(doc); err != nil {
		t.Fatal(err)
	}
	transport := &gateTransport{
		EnterSend: make(chan struct{}, 1),
		Proceed:   make(chan struct{}),
	}
	var wg sync.WaitGroup
	wg.Add(1)
	var d1 int
	var d1Err error
	go func() {
		defer wg.Done()
		d1, d1Err = c1.DeliverNotifications("lab", transport, now)
	}()
	<-transport.EnterSend
	d2, d2Err := c2.DeliverNotifications("lab", &outbox.FakeTransport{}, now)
	if !errors.Is(d2Err, ErrDispatchBusy) || d2 != 0 {
		t.Fatalf("second dispatcher: n=%d err=%v", d2, d2Err)
	}
	close(transport.Proceed)
	wg.Wait()
	if d1Err != nil || d1 != 1 {
		t.Fatalf("first dispatcher: n=%d err=%v", d1, d1Err)
	}
	if transport.sent.Load() != 1 {
		t.Fatalf("transport sends=%d", transport.sent.Load())
	}
}

func TestConcurrentDispatchAtLeastOnceAfterSendWithoutAck(t *testing.T) {
	now := time.Unix(1730000000, 0).UTC()
	ev := monitor.Event{Key: testHostKey(), Kind: monitor.Alert, Level: monitor.Critical, EmittedUnix: 100}
	q := &outbox.Queue{}
	if err := q.EnqueueDeduped([]monitor.Event{ev}, now); err != nil {
		t.Fatal(err)
	}
	fake := &outbox.FakeTransport{}
	msg, err := outbox.FormatMessage("lab", ev)
	if err != nil {
		t.Fatal(err)
	}
	if err := fake.Send(nil, msg); err != nil {
		t.Fatal(err)
	}
	n, err := outbox.DeliverQueue(q, "lab", fake, now)
	if err != nil || n != 1 || len(fake.Sent) != 2 {
		t.Fatalf("retry after crash-before-ack: n=%d sent=%d err=%v", n, len(fake.Sent), err)
	}
}

func testHostKey() string {
	k, _ := monitor.Key("host", "concurrency-test")
	return k
}
