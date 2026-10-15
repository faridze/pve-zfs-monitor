package device

import (
	"testing"
)

func TestParseSmartctlNVMeHealthy(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_nvme_healthy.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.SmartPassed == nil || !*h.SmartPassed {
		t.Fatalf("smart_passed: %#v", h.SmartPassed)
	}
	if h.PercentageUsed == nil || h.PercentageUsed.Value == nil || *h.PercentageUsed.Value != 3 {
		t.Fatalf("percentage_used: %#v", h.PercentageUsed)
	}
}

func TestParseSmartctlCriticalExitPreservesFailure(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_critical_exit.json"), 64)
	if err != nil {
		t.Fatal(err)
	}
	if h.SmartPassed == nil || *h.SmartPassed {
		t.Fatalf("must not default to healthy: %#v", h.SmartPassed)
	}
	if h.CriticalWarning == nil || *h.CriticalWarning == 0 {
		t.Fatalf("critical_warning: %#v", h.CriticalWarning)
	}
}

func TestParseSmartctlMalformedJSON(t *testing.T) {
	_, err := ParseSmartctlJSON([]byte("{"), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUnknownCollectionDoesNotImplyHealthy(t *testing.T) {
	h := DeviceHealth{CollectionStatus: HealthMissingTool}
	if h.SmartPassed != nil {
		t.Fatal("missing tool must not set smart_passed")
	}
}
