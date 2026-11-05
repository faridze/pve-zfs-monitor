package labrun

import (
	"testing"
)

func TestValidateLabStateDirRejectsRelative(t *testing.T) {
	if err := ValidateLabStateDir("relative/path"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateLabStateDirRejectsMissing(t *testing.T) {
	if err := ValidateLabStateDir("/tmp/pve-zfs-monitor-nonexistent-lab-dir-xyz"); err == nil {
		t.Fatal("expected missing dir error")
	}
}

func TestParseDeliveryMode(t *testing.T) {
	if _, err := ParseDeliveryMode("smtp"); err == nil {
		t.Fatal("expected error")
	}
	if m, err := ParseDeliveryMode("none"); err != nil || m != DeliveryNone {
		t.Fatalf("none: %v", err)
	}
}
