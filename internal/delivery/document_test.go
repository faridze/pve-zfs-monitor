package delivery

import (
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
	"github.com/faridze/pve-zfs-monitor/internal/persistjson"
)

func TestEncodeDecodeRoundTripStrictJSON(t *testing.T) {
	doc := emptyFirstRunDocument()
	doc.State = monitor.State{Version: monitor.StateVersion(), Records: map[string]monitor.Record{}}
	data, err := encodeDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := persistjson.ValidateFormatV2Envelope(data); err != nil {
		t.Fatalf("validate: %v data=%s", err, string(data))
	}
	if _, err := decodeDocument(data); err != nil {
		t.Fatal(err)
	}
}

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
	if doc.Outbox.Entries == nil {
		t.Fatal("expected explicit empty entries slice")
	}
}

func TestDecodeV2MissingOutboxEntriesMigratesEmptyQueue(t *testing.T) {
	raw := `{"format_version":2,"state":{"version":1,"last_run_unix":1730000000,"records":{}},"outbox":{"version":1}}`
	doc, err := decodeDocument([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Outbox.Entries) != 0 {
		t.Fatalf("expected empty queue, got %d", len(doc.Outbox.Entries))
	}
}

func TestDecodeV2NullOutboxEntriesRejected(t *testing.T) {
	raw := `{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1,"entries":null}}`
	if _, err := decodeDocument([]byte(raw)); err == nil {
		t.Fatal("expected null entries rejection")
	}
}
