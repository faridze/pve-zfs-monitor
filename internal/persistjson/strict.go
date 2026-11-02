package persistjson

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// ValidateSingleObject rejects duplicate keys and trailing JSON after the first value.
func ValidateSingleObject(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	if err := readObject(dec); err != nil {
		return err
	}
	var junk json.RawMessage
	err := dec.Decode(&junk)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return errors.New("trailing json document")
	}
	return fmt.Errorf("trailing json document: %w", err)
}

func readObject(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	d, ok := t.(json.Delim)
	if !ok || d != '{' {
		return errors.New("expected json object")
	}
	return readObjectContents(dec)
}

func readObjectContents(dec *json.Decoder) error {
	seen := map[string]bool{}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return errors.New("invalid object key")
		}
		if seen[key] {
			return fmt.Errorf("duplicate json key %q", key)
		}
		seen[key] = true
		if err := skipValue(dec); err != nil {
			return err
		}
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	return nil
}

func skipValue(dec *json.Decoder) error {
	t, err := dec.Token()
	if err != nil {
		return err
	}
	switch v := t.(type) {
	case json.Delim:
		switch v {
		case '{':
			return readObjectContents(dec)
		case '[':
			for dec.More() {
				if err := skipValue(dec); err != nil {
					return err
				}
			}
			_, err := dec.Token()
			return err
		}
	}
	return nil
}

// ValidateOutboxObject validates outbox JSON object shape.
func ValidateOutboxObject(raw []byte) error {
	if len(raw) == 0 {
		return errors.New("missing outbox")
	}
	if err := ValidateSingleObject(raw); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	for key := range fields {
		switch key {
		case "version", "entries":
		default:
			return fmt.Errorf("unknown outbox field %q", key)
		}
	}
	return nil
}

// ValidateLegacyStateObject validates P2-B legacy state JSON.
func ValidateLegacyStateObject(data []byte) error {
	if err := ValidateSingleObject(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]bool{
		"version": true, "last_run_unix": true, "records": true, "baselines": true,
	}
	for key := range fields {
		if !allowed[key] {
			return fmt.Errorf("unknown legacy state field %q", key)
		}
	}
	return nil
}

// ValidateFormatV2Envelope validates the top-level v2 document envelope.
func ValidateFormatV2Envelope(data []byte) error {
	if err := ValidateSingleObject(data); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for key := range fields {
		switch key {
		case "format_version", "state", "outbox":
		default:
			return fmt.Errorf("unknown document field %q", key)
		}
	}
	if stateRaw, ok := fields["state"]; ok {
		if err := ValidateSingleObject(stateRaw); err != nil {
			return fmt.Errorf("state: %w", err)
		}
	}
	if outboxRaw, ok := fields["outbox"]; ok {
		if err := ValidateOutboxObject(outboxRaw); err != nil {
			return err
		}
	}
	return nil
}
