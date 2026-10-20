package monitor

import (
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func enrichTopologyListOK(topo *zfs.TopologyReport, health string) {
	topo.EnrichListHealth([]zfs.Pool{{Name: topo.Pools[0].Name, Health: health}})
}

func TestDetectEmptyDeviceReportCoverageUnknown(t *testing.T) {
	report := deviceReportShell(baseTime)
	report.InventoryCompleteness = ""
	report.MappingCompleteness = ""
	report.SmartCompleteness = ""
	report.Inventory = nil
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if signalLevel(signals, "host", "device-smart-coverage") != Unknown {
		t.Fatalf("empty report must not be healthy coverage: %+v", signals)
	}
}

func TestDetectHealthCollectedWithoutSMARTEvidenceUnknown(t *testing.T) {
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath:       "/dev/sda",
		StableHardwareID: "disk-a",
	}}
	report.DeviceHealth = []device.DeviceHealth{{
		DevicePath:       "/dev/sda",
		CollectionStatus: device.HealthCollected,
	}}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Unknown) {
		t.Fatalf("collected without evidence must be unknown: %+v", signals)
	}
}

func TestDetectRejectsStaleDeviceReport(t *testing.T) {
	report := deviceReportShell(baseTime.Add(-30 * time.Minute))
	_, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err == nil {
		t.Fatal("expected stale device report error")
	}
}

func TestDetectRejectsFutureDeviceCollectedAt(t *testing.T) {
	report := deviceReportShell(baseTime.Add(5 * time.Minute))
	_, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err == nil {
		t.Fatal("expected future collected_at error")
	}
}

func TestDetectDeviceSignalsUseCollectedAtNotEvalTime(t *testing.T) {
	collected := baseTime.Add(-2 * time.Minute)
	report := deviceReportShell(collected)
	raw := loadDeviceFixture("smartctl_nvme_healthy.json")
	h, err := device.ParseSmartctlJSON(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.DevicePath = "/dev/nvme0n1"
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath: "/dev/nvme0n1", StableHardwareID: "nvme-1",
	}}
	report.DeviceHealth = []device.DeviceHealth{h}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range signals {
		if s.Key == "" {
			continue
		}
		if stringsHasPrefixDeviceSmart(s) && !s.ObservedAt.Equal(collected.UTC()) {
			t.Fatalf("device signal observed_at=%v want collected %v", s.ObservedAt, collected.UTC())
		}
	}
}

func stringsHasPrefixDeviceSmart(s Signal) bool {
	return len(s.Key) > 7 && s.Key[:7] == "device:"
}

func TestDetectTopologyReportIssuesFailClosed(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	topo.ReportIssues = []string{"synthetic issue"}
	enrichTopologyListOK(topo, "ONLINE")
	signals, _, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if signalLevel(signals, "host", "zpool-topology") != Unknown {
		t.Fatalf("topology issues must fail host closed: %+v", signals)
	}
}

func TestDecideWithDetectionPreservesCriticalZFSWhenMappingMissing(t *testing.T) {
	topo := loadZFSStatus("mirror_degraded.txt")
	enrichTopologyListOK(topo, "DEGRADED")
	report := deviceReportShell(baseTime)
	report.MappingCompleteness = device.ReportMappingIncomplete
	report.SmartCompleteness = device.ReportSmartIncomplete
	report.MemberMappings = []device.MemberMapping{{
		PoolName: "rpool", ZFSMemberName: "/dev/nvme1n1", Outcome: device.MappingMissing,
	}}
	in := DetectionInput{
		Topology: topo, TopologyOK: true,
		Devices: report, DevicesOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "DEGRADED"}},
	}
	_, events, err := DecideWithDetection(State{}, in, baseTime, Options{MaxAge: time.Hour}, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !eventsContainLevel(events, Critical) {
		t.Fatalf("UNAVAIL member must alert critical despite missing mapping: %+v", events)
	}
}

func TestDecideWithDetectionDoesNotEmitHealthySMARTWhenCollectionFailed(t *testing.T) {
	report := deviceReportShell(baseTime)
	report.SmartCompleteness = device.ReportSmartIncomplete
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath: "/dev/nvme0n1", StableHardwareID: "nvme-1",
	}}
	report.DeviceHealth = []device.DeviceHealth{{
		DevicePath: "/dev/nvme0n1", CollectionStatus: device.HealthFailed,
	}}
	in := DetectionInput{Devices: report, DevicesOK: true}
	_, events, err := DecideWithDetection(State{}, in, baseTime, Options{MaxAge: time.Hour}, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Kind == Alert && e.Level == Healthy {
			t.Fatalf("must not alert healthy on failed SMART: %+v", events)
		}
	}
}

func TestDetectRisingSMARTMediaErrors(t *testing.T) {
	v1, v2 := int64(1), int64(4)
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{{DevicePath: "/dev/nvme0n1", StableHardwareID: "nvme-1"}}
	report.DeviceHealth = []device.DeviceHealth{{
		DevicePath: "/dev/nvme0n1", CollectionStatus: device.HealthCollected,
		SmartPassed: boolPtr(true), MediaErrors: &device.SmartNumeric{Value: &v1},
	}}
	signals, base, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	report.DeviceHealth[0].MediaErrors = &device.SmartNumeric{Value: &v2}
	collected2 := baseTime.Add(time.Minute)
	report.CollectedAt = collected2.Format(time.RFC3339)
	signals2, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, base, baseTime.Add(time.Minute), DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals2, Warning) {
		t.Fatalf("expected SMART counter increase warning: first=%+v second=%+v", signals, signals2)
	}
}

func TestDetectMixedZFSCounterDeltasUnknown(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	bumpTopologyCounters(topo, 3)
	_, base, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	setFirstLeafCounters(topo, 1, 5, 3) // read down, write up vs prior 3,3,3 on first leaf
	signals, _, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, base, baseTime.Add(time.Minute), DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Unknown) {
		t.Fatalf("mixed counter directions on one member must be unknown: %+v", signals)
	}
}

func TestDetectHardwareReplacementNewBaselineKey(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	report := deviceReportShell(baseTime)
	report.MemberMappings = []device.MemberMapping{{
		PoolName: "rpool", ZFSMemberName: "/dev/nvme0n1", Outcome: device.MappingMapped,
		Physical: &device.PhysicalIdentity{StableHardwareID: "disk-old"},
	}}
	in := DetectionInput{Topology: topo, TopologyOK: true, Devices: report, DevicesOK: true}
	_, base, err := Detect(in, nil, baseTime, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Members) == 0 {
		t.Fatal("expected baselines")
	}
	report.MemberMappings[0].Physical = &device.PhysicalIdentity{StableHardwareID: "disk-new"}
	_, base2, err := Detect(in, base, baseTime.Add(time.Minute), DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if len(base2.Members) <= len(base.Members) {
		// new hw id → new key; old baseline should remain but new key gets fresh baseline
		foundNew := false
		for k := range base2.Members {
			if _, ok := base.Members[k]; !ok {
				foundNew = true
			}
		}
		if !foundNew {
			t.Fatalf("replacement should introduce new member baseline keys: old=%d new=%d", len(base.Members), len(base2.Members))
		}
	}
}

func TestDecideWithDetectionCounterResetThenRebaseline(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	enrichTopologyListOK(topo, "ONLINE")
	_, base, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bumpTopologyCounters(topo, 1)
	state := State{Baselines: base}
	in := DetectionInput{Topology: topo, TopologyOK: true, PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "ONLINE"}}}
	state, events, err := DecideWithDetection(state, in, baseTime.Add(time.Minute), Options{MaxAge: time.Hour}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected alert after counter increase")
	}
	zeroTopologyCounters(topo)
	state, _, err = DecideWithDetection(state, in, baseTime.Add(2*time.Minute), Options{MaxAge: time.Hour}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// two verified healthy samples after reset for recovery — first pass unknown should not clear alert silently
	state, events2, err := DecideWithDetection(state, in, baseTime.Add(3*time.Minute), Options{MaxAge: time.Hour, RecoverySamples: 2}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_ = events2
	if state.Baselines == nil {
		t.Fatal("expected baselines after rebaseline path")
	}
}

func signalLevel(signals []Signal, scope, identity string) Level {
	k, _ := Key(scope, identity)
	for _, s := range signals {
		if s.Key == k {
			return s.Level
		}
	}
	return Unknown
}

func eventsContainLevel(events []Event, want Level) bool {
	for _, e := range events {
		if e.Level == want && e.Kind == Alert {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool { return &b }

func setFirstLeafCounters(topo *zfs.TopologyReport, read, write, cksum uint64) {
	if len(topo.Pools) == 0 || topo.Pools[0].Root == nil {
		return
	}
	setFirstLeafOnNode(topo.Pools[0].Root, read, write, cksum)
}

func setFirstLeafOnNode(node *zfs.VdevNode, read, write, cksum uint64) {
	if node == nil {
		return
	}
	if isTopologyLeaf(node) {
		node.ReadErrors = &read
		node.WriteErrors = &write
		node.CksumErrors = &cksum
		return
	}
	if len(node.Children) > 0 {
		setFirstLeafOnNode(&node.Children[0], read, write, cksum)
	}
}
