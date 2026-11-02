package persistjson

import (
	"strings"
	"testing"
)

func TestValidateLegacyRejectsMisspelledEntries(t *testing.T) {
	raw := []byte(`{"version":1,"entires":[]}`)
	err := ValidateLegacyStateObject(raw)
	if err == nil {
		t.Fatal("expected rejection of entires typo")
	}
	if !strings.Contains(err.Error(), "entires") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateFormatV2RejectsDuplicateTopLevelKeys(t *testing.T) {
	raw := []byte(`{"format_version":2,"state":{"version":1,"records":{}},"state":{"version":1,"records":{}},"outbox":{"version":1}}`)
	if err := ValidateFormatV2Envelope(raw); err == nil {
		t.Fatal("expected duplicate state key rejection")
	}
}

func TestValidateFormatV2RejectsTrailingDocument(t *testing.T) {
	doc1 := `{"format_version":2,"state":{"version":1,"records":{}},"outbox":{"version":1}}`
	raw := []byte(doc1 + doc1)
	if err := ValidateFormatV2Envelope(raw); err == nil {
		t.Fatal("expected trailing document rejection")
	}
}

func TestValidateOutboxRejectsUnknownField(t *testing.T) {
	raw := []byte(`{"version":1,"entires":[]}`)
	if err := ValidateOutboxObject(raw); err == nil {
		t.Fatal("expected unknown outbox field rejection")
	}
}

func TestValidateOutboxAllowsMissingEntriesMigration(t *testing.T) {
	raw := []byte(`{"version":1}`)
	if err := ValidateOutboxObject(raw); err != nil {
		t.Fatal(err)
	}
}
