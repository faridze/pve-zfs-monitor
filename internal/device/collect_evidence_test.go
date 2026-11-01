package device

import "testing"

func TestHasSMARTCollectionEvidenceFailClosed(t *testing.T) {
	passed := true
	completeNoRows := &DeviceReport{
		InventoryCompleteness: ReportInventoryComplete,
		MappingCompleteness:   ReportMappingComplete,
		SmartCompleteness:     ReportSmartComplete,
	}
	if HasSMARTCollectionEvidence(completeNoRows) {
		t.Fatal("flags without inventory/health must not prove collection")
	}
	withEvidence := &DeviceReport{
		InventoryCompleteness: ReportInventoryComplete,
		MappingCompleteness:   ReportMappingComplete,
		SmartCompleteness:     ReportSmartComplete,
		Inventory:             []PhysicalIdentity{{StableHardwareID: "disk-a"}},
		DeviceHealth:          []DeviceHealth{{SmartPassed: &passed}},
	}
	if !HasSMARTCollectionEvidence(withEvidence) {
		t.Fatal("expected evidence with health rows")
	}
}
