package device

import "testing"

func TestMappingCompletenessRequiresAllMapped(t *testing.T) {
	mixed := []MemberMapping{
		{Outcome: MappingMapped, DevicePath: "/dev/sda"},
		{Outcome: MappingMissing},
	}
	if mappingCompleteness(mixed) != ReportMappingIncomplete {
		t.Fatal("expected incomplete for mixed mapping outcomes")
	}
	if mappingCompleteness([]MemberMapping{
		{Outcome: MappingMapped},
		{Outcome: MappingMapped},
	}) != ReportMappingComplete {
		t.Fatal("expected complete when all mapped")
	}
	if mappingCompleteness(nil) != ReportMappingIncomplete {
		t.Fatal("empty mappings must be incomplete")
	}
}

func TestSmartCompletenessRequiresAllCollected(t *testing.T) {
	expected := []string{"/dev/sda", "/dev/sdb", "/dev/sdc", "/dev/sdd"}
	oneOK := []DeviceHealth{
		{DevicePath: "/dev/sda", CollectionStatus: HealthCollected},
		{DevicePath: "/dev/sdb", CollectionStatus: HealthPermissionDenied},
		{DevicePath: "/dev/sdc", CollectionStatus: HealthFailed},
		{DevicePath: "/dev/sdd", CollectionStatus: HealthPartial},
	}
	if smartCompleteness(expected, oneOK) != ReportSmartIncomplete {
		t.Fatal("one success must not imply smart_complete")
	}
	conflict := []DeviceHealth{
		{DevicePath: "/dev/sda", CollectionStatus: HealthCollected},
		{DevicePath: "/dev/sdb", CollectionStatus: HealthConflict},
	}
	if smartCompleteness([]string{"/dev/sda", "/dev/sdb"}, conflict) != ReportSmartIncomplete {
		t.Fatal("health_conflict must not count as complete")
	}
	all := []DeviceHealth{
		{DevicePath: "/dev/sda", CollectionStatus: HealthCollected},
		{DevicePath: "/dev/sdb", CollectionStatus: HealthCollected},
	}
	if smartCompleteness([]string{"/dev/sda", "/dev/sdb"}, all) != ReportSmartComplete {
		t.Fatal("expected complete when all collected")
	}
	if smartCompleteness([]string{"/dev/sda"}, []DeviceHealth{}) != ReportSmartIncomplete {
		t.Fatal("missing health rows must be incomplete")
	}
}
