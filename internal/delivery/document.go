package delivery

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
)

const formatVersion2 = 2

// Document is the atomic on-disk bundle (monitor state + notification outbox).
type Document struct {
	FormatVersion int             `json:"format_version"`
	State         monitor.State   `json:"state"`
	Outbox        outbox.Snapshot `json:"outbox"`
}

func emptyFirstRunDocument() Document {
	return Document{
		FormatVersion: formatVersion2,
		Outbox:        outbox.Snapshot{Version: outbox.SnapshotVersion},
	}
}

// decodeDocument parses persisted bytes. data==nil means absent file (valid first run).
func decodeDocument(data []byte) (Document, error) {
	if data == nil {
		return emptyFirstRunDocument(), nil
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return Document{}, errors.New("empty state file")
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return Document{}, errors.New("null state file")
	}
	if trimmed[0] != '{' {
		return Document{}, errors.New("corrupt document: invalid JSON")
	}

	var meta struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(trimmed, &meta); err != nil {
		return Document{}, fmt.Errorf("corrupt document: %w", err)
	}
	if meta.FormatVersion == formatVersion2 {
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.DisallowUnknownFields()
		var doc Document
		if err := dec.Decode(&doc); err != nil {
			return Document{}, fmt.Errorf("corrupt document: %w", err)
		}
		return doc, validateDocument(doc, true)
	}
	if meta.FormatVersion != 0 {
		return Document{}, errors.New("unsupported document format")
	}
	// Legacy P2-B state file (no format_version).
	if bytes.Equal(trimmed, []byte("{}")) {
		return Document{}, errors.New("invalid legacy state")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var st monitor.State
	if err := dec.Decode(&st); err != nil {
		return Document{}, fmt.Errorf("corrupt legacy state: %w", err)
	}
	if st.Version != monitor.StateVersion() {
		return Document{}, fmt.Errorf("invalid legacy state version %d", st.Version)
	}
	if err := monitor.ValidatePersistedState(st); err != nil {
		return Document{}, err
	}
	return Document{
		FormatVersion: formatVersion2,
		State:         st,
		Outbox:        outbox.Snapshot{Version: outbox.SnapshotVersion},
	}, nil
}

func validateDocument(doc Document, persisted bool) error {
	if doc.FormatVersion != formatVersion2 {
		return errors.New("unsupported document format")
	}
	if persisted && doc.State.Version == 0 {
		return errors.New("missing state version in persisted document")
	}
	if doc.State.Version != 0 {
		if err := monitor.ValidatePersistedState(doc.State); err != nil {
			return err
		}
	}
	if doc.Outbox.Version == 0 {
		if persisted {
			return errors.New("missing outbox version")
		}
		doc.Outbox.Version = outbox.SnapshotVersion
	}
	q := &outbox.Queue{}
	return q.LoadSnapshot(doc.Outbox)
}

func encodeDocument(doc Document) ([]byte, error) {
	doc.FormatVersion = formatVersion2
	if doc.Outbox.Version == 0 {
		doc.Outbox.Version = outbox.SnapshotVersion
	}
	if err := validateDocument(doc, false); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}
