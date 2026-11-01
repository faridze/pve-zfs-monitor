package outbox

import (
	"bytes"
	"encoding/json"
	"errors"
)

// DecodePersistedSnapshot validates on-disk outbox JSON.
// Missing "entries" on a persisted document is treated as an empty queue (migration).
// Null entries is corrupt.
func DecodePersistedSnapshot(raw json.RawMessage, persisted bool) (Snapshot, error) {
	if len(raw) == 0 {
		if persisted {
			return Snapshot{}, errors.New("missing outbox")
		}
		return Snapshot{Version: snapshotVersion, Entries: []Entry{}}, nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Snapshot{}, errInvalidSnapshot
	}
	versionRaw, ok := fields["version"]
	if !ok {
		return Snapshot{}, errors.New("missing outbox version")
	}
	var version int
	if err := json.Unmarshal(versionRaw, &version); err != nil || version != snapshotVersion {
		return Snapshot{}, errInvalidSnapshot
	}
	entriesRaw, hasEntries := fields["entries"]
	if !hasEntries {
		if persisted {
			return Snapshot{Version: snapshotVersion, Entries: []Entry{}}, nil
		}
		return Snapshot{Version: snapshotVersion, Entries: []Entry{}}, nil
	}
	if bytes.Equal(bytes.TrimSpace(entriesRaw), []byte("null")) {
		return Snapshot{}, errors.New("null outbox entries")
	}
	var snap Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		return Snapshot{}, errInvalidSnapshot
	}
	q := &Queue{}
	if err := q.LoadSnapshot(snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}
