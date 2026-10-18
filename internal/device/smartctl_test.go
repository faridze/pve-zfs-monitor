package device

import (
	"testing"
)

func TestParseSmartctlNVMeHealthy(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_nvme_healthy.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.CollectionStatus != HealthCollected {
		t.Fatalf("status=%q", h.CollectionStatus)
	}
	if h.SmartPassed == nil || !*h.SmartPassed {
		t.Fatalf("smart_passed: %#v", h.SmartPassed)
	}
	if h.PercentageUsed == nil || h.PercentageUsed.Value == nil || *h.PercentageUsed.Value != 3 {
		t.Fatalf("percentage_used: %#v", h.PercentageUsed)
	}
}

func TestParseSmartctlPassedFalseWithoutStatusString(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_passed_false_only.json"), 8)
	if err != nil {
		t.Fatal(err)
	}
	if h.SmartPassed == nil || *h.SmartPassed {
		t.Fatalf("expected explicit false, got %#v", h.SmartPassed)
	}
}

func TestParseSmartctlPassedTrueExit8Conflict(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_passed_true_exit8.json"), 8)
	if err != nil {
		t.Fatal(err)
	}
	if h.CollectionStatus != HealthConflict {
		t.Fatalf("status=%q detail=%q", h.CollectionStatus, h.CollectionDetail)
	}
	if h.SmartPassed == nil || *h.SmartPassed {
		t.Fatalf("must not confirm healthy: %#v", h.SmartPassed)
	}
}

func TestParseATATemperatureUsesRawNotNormalizedValue(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_ata_temperature.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.Temperature == nil || h.Temperature.Value == nil || *h.Temperature.Value != 33 {
		t.Fatalf("temperature: %#v", h.Temperature)
	}
}

func TestParseSmartctlOperationalExitPartial(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_sparse_temp_exit4.json"), 4)
	if err != nil {
		t.Fatal(err)
	}
	if h.CollectionStatus != HealthPartial {
		t.Fatalf("status=%q", h.CollectionStatus)
	}
}

func TestParseSmartctlSparseNVMeNoFabricatedZeros(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_sparse_nvme.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.CriticalWarning != nil || h.PercentageUsed != nil || h.MediaErrors != nil {
		t.Fatalf("fabricated zeros: cw=%v pct=%v media=%v", h.CriticalWarning, h.PercentageUsed, h.MediaErrors)
	}
	if h.Temperature == nil || h.Temperature.Value == nil || *h.Temperature.Value != 40 {
		t.Fatalf("temperature: %#v", h.Temperature)
	}
}

func TestParseSmartctlNVMeAllZeroObserved(t *testing.T) {
	h, err := ParseSmartctlJSON(loadFixture("smartctl_nvme_all_zero.json"), 0)
	if err != nil {
		t.Fatal(err)
	}
	if h.Temperature == nil || h.Temperature.Value == nil || *h.Temperature.Value != 0 {
		t.Fatalf("temperature zero must be preserved: %#v", h.Temperature)
	}
	if h.PercentageUsed == nil || h.PercentageUsed.Value == nil {
		t.Fatal("percentage_used should be present in full log")
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

func TestSmartctlExitBitClassification(t *testing.T) {
	if SmartctlExitIndicatesHealthFailure(8) != true {
		t.Fatal("exit 8 must be health failure")
	}
	if SmartctlExitIndicatesHealthFailure(16) != true {
		t.Fatal("exit 16 must be health failure")
	}
	if SmartctlExitIndicatesOperationalError(2) != true {
		t.Fatal("exit 2 must be operational")
	}
	if SmartctlExitIndicatesHealthFailure(2) {
		t.Fatal("exit 2 is not disk health failure")
	}
	if SmartctlExitIndicatesHealthFailure(64) {
		t.Fatal("exit 64 is self-test log bit, not failing-now")
	}
}

func TestParseSmartctlMalformedJSON(t *testing.T) {
	_, err := ParseSmartctlJSON([]byte("{"), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSmartctlErrorOnlyJSONRejected(t *testing.T) {
	_, err := ParseSmartctlJSON(loadFixture("smartctl_error_only.json"), 2)
	if err == nil {
		t.Fatal("expected no health evidence error")
	}
}

func TestUnknownCollectionDoesNotImplyHealthy(t *testing.T) {
	h := DeviceHealth{CollectionStatus: HealthMissingTool}
	if h.SmartPassed != nil {
		t.Fatal("missing tool must not set smart_passed")
	}
}
