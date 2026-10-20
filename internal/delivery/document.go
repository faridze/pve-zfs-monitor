package delivery

import (
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

func decodeDocument(data []byte) (Document, error) {
	if len(data) == 0 {
		return Document{FormatVersion: formatVersion2, Outbox: outbox.Snapshot{Version: 1}}, nil
	}
	var meta struct {
		FormatVersion int `json:"format_version"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return Document{}, fmt.Errorf("corrupt document: %w", err)
	}
	if meta.FormatVersion == formatVersion2 {
		var doc Document
		if err := json.Unmarshal(data, &doc); err != nil {
			return Document{}, fmt.Errorf("corrupt document: %w", err)
		}
		return doc, validateDocument(doc)
	}
	var st monitor.State
	if err := json.Unmarshal(data, &st); err != nil {
		return Document{}, fmt.Errorf("corrupt legacy state: %w", err)
	}
	if st.Version != 0 {
		if err := monitor.ValidatePersistedState(st); err != nil {
			return Document{}, err
		}
	}
	return Document{
		FormatVersion: formatVersion2,
		State:         st,
		Outbox:        outbox.Snapshot{Version: 1},
	}, nil
}

func validateDocument(doc Document) error {
	if doc.FormatVersion != formatVersion2 {
		return errors.New("unsupported document format")
	}
	if doc.State.Version != 0 {
		if err := monitor.ValidatePersistedState(doc.State); err != nil {
			return err
		}
	}
	q := &outbox.Queue{}
	return q.LoadSnapshot(doc.Outbox)
}

func encodeDocument(doc Document) ([]byte, error) {
	doc.FormatVersion = formatVersion2
	if doc.Outbox.Version == 0 {
		doc.Outbox.Version = 1
	}
	if err := validateDocument(doc); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}
