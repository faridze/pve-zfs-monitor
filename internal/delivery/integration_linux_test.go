//go:build linux

package delivery

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func privateDir(t *testing.T) string {
	d := t.TempDir()
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCoordinatorAdvanceAndDeliverSurvivesRestart(t *testing.T) {
	dir := privateDir(t)
	c := Coordinator{Store: Store{Directory: dir}}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	in := monitor.DetectionInput{
		PoolsOK: true,
		Pools:   []zfs.Pool{{Name: "p1", Health: "DEGRADED"}},
	}
	_, events, err := c.AdvanceDetection(in, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{})
	if err != nil || len(events) == 0 {
		t.Fatalf("advance: events=%+v err=%v", events, err)
	}
	c2 := Coordinator{Store: Store{Directory: dir}}
	pending, err := c2.PendingNotifications()
	if err != nil || pending == 0 {
		t.Fatalf("pending=%d err=%v", pending, err)
	}
	fake := &outbox.FakeTransport{}
	n, err := c2.DeliverNotifications("lab-node", fake, now)
	if err != nil || n == 0 || len(fake.Sent) == 0 {
		t.Fatalf("deliver n=%d err=%v sent=%d", n, err, len(fake.Sent))
	}
	doc, err := c2.Store.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range doc.Outbox.Entries {
		if e.Status != outbox.StatusDelivered {
			t.Fatalf("expected all delivered after ack: %+v", e)
		}
	}
}

func TestLegacyStateMigration(t *testing.T) {
	dir := privateDir(t)
	legacy := `{"version":1,"last_run_unix":1730000000,"records":{}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := Store{Directory: dir}.Load()
	if err != nil {
		t.Fatal(err)
	}
	if doc.FormatVersion != formatVersion2 || doc.State.Version != 1 {
		t.Fatalf("migration: %+v", doc)
	}
}

func TestCorruptDocumentFailsClosed(t *testing.T) {
	dir := privateDir(t)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"format_version":2,"state":{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Store{Directory: dir}.Load()
	if err == nil {
		t.Fatal("expected corrupt load failure")
	}
}

func TestConcurrentWriterLockBusy(t *testing.T) {
	dir := privateDir(t)
	s := Store{Directory: dir}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = s.withLock(func(dir int) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
	}()
	time.Sleep(20 * time.Millisecond)
	err := s.Save(Document{FormatVersion: formatVersion2, Outbox: outbox.Snapshot{Version: 1}})
	if err == nil || !errors.Is(err, errors.New("state lock busy")) {
		// error message contains busy
		if err == nil || err.Error() != "state lock busy: resource temporarily unavailable" {
			// accept any busy error
			if err == nil || !contains(err.Error(), "busy") {
				t.Fatalf("expected busy, got %v", err)
			}
		}
	}
	wg.Wait()
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
