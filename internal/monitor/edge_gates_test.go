package monitor

import (
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestDecideWithDetectionCriticalZFSWithMissingCountersAndMapping(t *testing.T) {
	topo := loadZFSStatus("mirror_degraded.txt")
	enrichTopologyListOK(topo, "DEGRADED")
	stripAllLeafCounters(topo)
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
		t.Fatalf("expected critical alert with missing counters and mapping: %+v", events)
	}
}

func TestDecideWithDetectionDoesNotInventCriticalFromUnknownZFSState(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	enrichTopologyListOK(topo, "ONLINE")
	setFirstLeafState(topo, "BOGUS", false)
	in := DetectionInput{Topology: topo, TopologyOK: true}
	_, events, err := DecideWithDetection(State{}, in, baseTime, Options{MaxAge: time.Hour}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		if e.Kind == Alert && e.Level == Critical {
			t.Fatalf("unobserved/unknown state must not critical alert: %+v", events)
		}
	}
}

func TestHardwareCollisionHealthyMembersFailClosed(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	report := deviceReportShell(baseTime)
	report.MappingCompleteness = device.ReportMappingIncomplete
	report.SmartCompleteness = device.ReportSmartIncomplete
	report.MemberMappings = []device.MemberMapping{
		{
			PoolName: "rpool", ZFSMemberName: "/dev/nvme0n1", Outcome: device.MappingMapped,
			Physical: &device.PhysicalIdentity{StableHardwareID: "shared-hw"},
		},
		{
			PoolName: "rpool", ZFSMemberName: "/dev/nvme1n1", Outcome: device.MappingMapped,
			Physical: &device.PhysicalIdentity{StableHardwareID: "shared-hw"},
		},
	}
	signals, _, err := Detect(DetectionInput{Topology: topo, TopologyOK: true, Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	healthyDevices := 0
	for _, s := range signals {
		if stringsHasPrefixDeviceSmart(s) && s.Level == Healthy && s.Verified {
			healthyDevices++
		}
	}
	if healthyDevices != 0 {
		t.Fatalf("hw collision must not yield verified healthy members: %+v", signals)
	}
}

func TestHardwareCollisionPreservesCriticalZFSIndependentOfOrder(t *testing.T) {
	run := func(swap bool) []Signal {
		topo := loadZFSStatus("mirror_degraded.txt")
		enrichTopologyListOK(topo, "DEGRADED")
		report := deviceReportShell(baseTime)
		report.MappingCompleteness = device.ReportMappingIncomplete
		report.SmartCompleteness = device.ReportSmartIncomplete
		m0 := device.MemberMapping{
			PoolName: "rpool", ZFSMemberName: "/dev/nvme0n1", Outcome: device.MappingMapped,
			Physical: &device.PhysicalIdentity{StableHardwareID: "shared-hw"},
		}
		m1 := device.MemberMapping{
			PoolName: "rpool", ZFSMemberName: "/dev/nvme1n1", Outcome: device.MappingMapped,
			Physical: &device.PhysicalIdentity{StableHardwareID: "shared-hw"},
		}
		if swap {
			report.MemberMappings = []device.MemberMapping{m1, m0}
		} else {
			report.MemberMappings = []device.MemberMapping{m0, m1}
		}
		signals, _, err := Detect(DetectionInput{Topology: topo, TopologyOK: true, Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{MaxReportAge: time.Hour})
		if err != nil {
			t.Fatal(err)
		}
		return signals
	}
	a := run(false)
	b := run(true)
	if countDeviceLevel(a, Critical) != countDeviceLevel(b, Critical) {
		t.Fatalf("order-dependent critical counts: %d vs %d\na=%+v\nb=%+v", countDeviceLevel(a, Critical), countDeviceLevel(b, Critical), a, b)
	}
	if countDeviceLevel(a, Critical) == 0 {
		t.Fatalf("expected preserved critical ZFS on collision: %+v", a)
	}
}

func TestDecideWithDetectionDuplicateSMARTIdentitySingleKey(t *testing.T) {
	rawBad := loadDeviceFixture("smartctl_critical_exit.json")
	hBad, err := device.ParseSmartctlJSON(rawBad, 64)
	if err != nil {
		t.Fatal(err)
	}
	hBad.DevicePath = "/dev/nvme0n1"
	rawOK := loadDeviceFixture("smartctl_nvme_healthy.json")
	hOK, err := device.ParseSmartctlJSON(rawOK, 0)
	if err != nil {
		t.Fatal(err)
	}
	hOK.DevicePath = "/dev/nvme0n1p3"
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{
		{DevicePath: "/dev/nvme0n1", StableHardwareID: "same-disk"},
		{DevicePath: "/dev/nvme0n1p3", StableHardwareID: "same-disk"},
	}
	report.DeviceHealth = []device.DeviceHealth{hOK, hBad}
	in := DetectionInput{Devices: report, DevicesOK: true}
	_, events, err := DecideWithDetection(State{}, in, baseTime, Options{MaxAge: time.Hour}, DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatalf("DecideWithDetection must not reject duplicate keys: %v", err)
	}
	if !eventsContainLevel(events, Critical) {
		t.Fatalf("duplicate SMART rows must preserve critical evidence: %+v", events)
	}
}

func TestHealthCollectedTemperatureOnlyNotVerifiedHealthy(t *testing.T) {
	temp := int64(42)
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath: "/dev/sda", StableHardwareID: "ata-1",
	}}
	report.DeviceHealth = []device.DeviceHealth{{
		DevicePath:         "/dev/sda",
		CollectionStatus: device.HealthCollected,
		Temperature:        &device.SmartNumeric{Value: &temp, Unit: "celsius"},
	}}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range signals {
		if stringsHasPrefixDeviceSmart(s) && s.Level == Healthy && s.Verified {
			t.Fatalf("temperature-only must not be verified healthy: %+v", signals)
		}
	}
}

func countDeviceLevel(signals []Signal, want Level) int {
	n := 0
	for _, s := range signals {
		if stringsHasPrefixDeviceSmart(s) && s.Level == want {
			n++
		}
	}
	return n
}

func stripAllLeafCounters(topo *zfs.TopologyReport) {
	for i := range topo.Pools {
		if topo.Pools[i].Root != nil {
			stripNodeCounters(topo.Pools[i].Root)
		}
	}
}

func stripNodeCounters(node *zfs.VdevNode) {
	if node == nil {
		return
	}
	if isTopologyLeaf(node) {
		node.ReadErrors = nil
		node.WriteErrors = nil
		node.CksumErrors = nil
		return
	}
	for i := range node.Children {
		stripNodeCounters(&node.Children[i])
	}
}

func setFirstLeafState(topo *zfs.TopologyReport, state string, observed bool) {
	if len(topo.Pools) == 0 || topo.Pools[0].Root == nil {
		return
	}
	setFirstLeafStateNode(topo.Pools[0].Root, state, observed)
}

func setFirstLeafStateNode(node *zfs.VdevNode, state string, observed bool) {
	if node == nil {
		return
	}
	if isTopologyLeaf(node) {
		node.State = state
		node.StateObserved = observed
		return
	}
	if len(node.Children) > 0 {
		setFirstLeafStateNode(&node.Children[0], state, observed)
	}
}
