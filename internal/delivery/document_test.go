package delivery

import (
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

func TestDecodeAbsentFileIsFirstRun(t *testing.T) {
	doc, err := decodeDocument(nil)
	if err != nil {
		t.Fatal(err)
	}
	if doc.State.Version != 0 || doc.FormatVersion != formatVersion2 {
		t.Fatalf("first run: %+v", doc)
	}
}

func TestDecodeRejectsEmptyAndNullOnDisk(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", "   "} {
		_, err := decodeDocument([]byte(raw))
		if err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestDecodeRejectsInvalidLegacyAndV2(t *testing.T) {
	cases := []string{
		`{"version":0,"records":{}}`,
		`{"format_version":2}`,
		`{"format_version":2,"state":{"version":1,"records":{}}}`,
		`{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1,"entries":[{"id":"short","logical_id":"bad","event":{"key":"x","kind":"alert","level":"critical"},"created_at_unix":1,"status":"pending"}]}}`,
		`{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1,"entries":[]},"extra":1}`,
	}
	for _, raw := range cases {
		if _, err := decodeDocument([]byte(raw)); err == nil {
			t.Fatalf("expected error for %s", raw)
		}
	}
}

func TestDecodeValidLegacyMigrates(t *testing.T) {
	raw := `{"version":1,"last_run_unix":1730000000,"records":{}}`
	doc, err := decodeDocument([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if doc.State.Version != monitor.StateVersion() || doc.Outbox.Version != outbox.SnapshotVersion {
		t.Fatalf("migrate: %+v", doc)
	}
}
