package outbox

import (
	"encoding/json"
	"testing"
)

func TestDecodePersistedSnapshotNullEntriesRejected(t *testing.T) {
	raw := json.RawMessage(`{"version":1,"entries":null}`)
	if _, err := DecodePersistedSnapshot(raw, true); err == nil {
		t.Fatal("expected null entries error")
	}
}

func TestDecodePersistedSnapshotMissingEntriesMigrates(t *testing.T) {
	raw := json.RawMessage(`{"version":1}`)
	snap, err := DecodePersistedSnapshot(raw, true)
	if err != nil || len(snap.Entries) != 0 {
		t.Fatalf("migrate empty: %+v err=%v", snap, err)
	}
}
