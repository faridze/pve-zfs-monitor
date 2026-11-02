//go:build linux

package delivery

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestCorruptPersistedFileNotOverwrittenOnLoadAdvanceDeliver(t *testing.T) {
	dir := privateDir(t)
	corrupt := []byte(`{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1,"entires":[]}}`)
	path := filepath.Join(dir, "state.json")
	if err := os.WriteFile(path, corrupt, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	store := Store{Directory: dir}
	if _, err := store.Load(); err == nil {
		t.Fatal("load should fail")
	}
	c := Coordinator{Store: store}
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	in := monitor.DetectionInput{PoolsOK: true, Pools: []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}}
	if _, _, err := c.AdvanceDetection(in, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{}); err == nil {
		t.Fatal("advance should fail")
	}
	if _, err := c.DeliverNotifications("lab", &outbox.FakeTransport{}, now); err == nil {
		t.Fatal("deliver should fail")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("corrupt state file was modified")
	}
}

func TestDecodeRejectsLegacyEntiresTypo(t *testing.T) {
	raw := `{"version":1,"entires":[]}`
	if _, err := decodeDocument([]byte(raw)); err == nil {
		t.Fatal("expected legacy typo rejection")
	}
}

func TestDecodeRejectsDuplicateOutboxKey(t *testing.T) {
	raw := `{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1,"outbox":{"version":1}}}`
	if _, err := decodeDocument([]byte(raw)); err == nil {
		t.Fatal("expected error")
	}
	// duplicate outbox field at envelope level
	raw2 := `{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1},"outbox":{"version":1}}`
	if _, err := decodeDocument([]byte(raw2)); err == nil {
		t.Fatal("expected duplicate outbox rejection")
	}
}

func TestDecodeRejectsConcatenatedDocuments(t *testing.T) {
	one := `{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1}}`
	if _, err := decodeDocument([]byte(one + one)); err == nil {
		t.Fatal("expected trailing document rejection")
	}
}
