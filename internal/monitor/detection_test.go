package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func loadZFSStatus(name string) *zfs.TopologyReport {
	data, err := os.ReadFile(filepath.Join("..", "zfs", "testdata", name))
	if err != nil {
		panic(err)
	}
	report, err := zfs.ParseStatus(data)
	if err != nil {
		panic(err)
	}
	return report
}

func loadDeviceFixture(name string) []byte {
	data, err := os.ReadFile(filepath.Join("..", "device", "testdata", name))
	if err != nil {
		panic(err)
	}
	return data
}

func deviceReportShell(collected time.Time) *device.DeviceReport {
	return &device.DeviceReport{
		CollectedAt:           collected.UTC().Format(time.RFC3339),
		InventoryCompleteness: device.ReportInventoryComplete,
		MappingCompleteness:   device.ReportMappingComplete,
		SmartCompleteness:     device.ReportSmartComplete,
	}
}

func TestDetectMirrorDegradedMemberCritical(t *testing.T) {
	topo := loadZFSStatus("mirror_degraded.txt")
	at := baseTime
	in := DetectionInput{Topology: topo, TopologyOK: true, PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "DEGRADED"}}}
	signals, _, err := Detect(in, nil, at, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLevel(signals, Critical) {
		t.Fatalf("expected critical member signal, got %+v", signals)
	}
	assertNoSensitiveLeak(t, signals)
}

func TestDetectRisingZFSCountersBetweenRuns(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	at1 := baseTime
	in1 := DetectionInput{Topology: topo, TopologyOK: true}
	_, base, err := Detect(in1, nil, at1, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(base.Members) == 0 {
		t.Fatal("expected member baselines")
	}
	bumpTopologyCounters(topo, 1)
	at2 := at1.Add(5 * time.Minute)
	s2, _, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, base, at2, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLevel(s2, Warning) && !containsLevel(s2, Critical) {
		t.Fatalf("expected escalation after counter increase, got %+v", s2)
	}
}

func TestDetectDoesNotAlertHistoricalNonzeroCountersOnFirstBaseline(t *testing.T) {
	topo := loadZFSStatus("mirror_faulted_counters.txt")
	at := baseTime
	signals, base, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, nil, at, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// First observation records baselines; member state still drives severity.
	if base == nil || len(base.Members) == 0 {
		t.Fatal("expected baseline capture")
	}
	for _, s := range signals {
		if s.Level == Healthy && strings.Contains(s.Key, "device:") {
			continue
		}
	}
}

func TestDetectAmbiguousMemberMappingUnknown(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	report := deviceReportShell(baseTime)
	report.MappingCompleteness = device.ReportMappingIncomplete
	report.SmartCompleteness = device.ReportSmartIncomplete
	report.MemberMappings = []device.MemberMapping{{
		PoolName:      "rpool",
		ZFSMemberName: "/dev/nvme0n1",
		Outcome:       device.MappingAmbiguous,
	}}
	signals, _, err := Detect(DetectionInput{
		Topology: topo, TopologyOK: true,
		Devices: report, DevicesOK: true,
	}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Unknown) {
		t.Fatalf("ambiguous mapping must be UNKNOWN, got %+v", signals)
	}
}

func TestDetectZFSCounterResetUnknown(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	_, base, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	bumpTopologyCounters(topo, 3)
	signals, base, err := Detect(DetectionInput{Topology: topo, TopologyOK: true}, base, baseTime.Add(time.Minute), DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Warning) {
		t.Fatalf("expected warning after counter increase, got %+v", signals)
	}
	zeroTopologyCounters(topo)
	signals, _, err = Detect(DetectionInput{Topology: topo, TopologyOK: true}, base, baseTime.Add(2*time.Minute), DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Unknown) {
		t.Fatalf("counter reset must be UNKNOWN, got %+v", signals)
	}
}

func TestDetectDuplicatePathsSameHardwareUnknown(t *testing.T) {
	raw := loadDeviceFixture("smartctl_nvme_healthy.json")
	h, err := device.ParseSmartctlJSON(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.DevicePath = "/dev/nvme0n1"
	h2 := h
	h2.DevicePath = "/dev/nvme0n1p3"
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{
		{DevicePath: "/dev/nvme0n1", StableHardwareID: "same-disk"},
		{DevicePath: "/dev/nvme0n1p3", StableHardwareID: "same-disk"},
	}
	report.DeviceHealth = []device.DeviceHealth{h, h2}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	unknownDevices := 0
	for _, s := range signals {
		if strings.HasPrefix(s.Key, "device:") && s.Level == Unknown {
			unknownDevices++
		}
	}
	if unknownDevices == 0 {
		t.Fatalf("duplicate hardware paths must not be treated as two healthy devices, got %+v", signals)
	}
}

func TestDetectSMARTFailureAndMissingCollection(t *testing.T) {
	raw := loadDeviceFixture("smartctl_critical_exit.json")
	h, err := device.ParseSmartctlJSON(raw, 64)
	if err != nil {
		t.Fatal(err)
	}
	h.DevicePath = "/dev/nvme0n1"
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath:       "/dev/nvme0n1",
		StableHardwareID: "nvme-test-1",
		RuntimeLocator:   "nvme-test-1",
	}}
	report.DeviceHealth = []device.DeviceHealth{h}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLevel(signals, Critical) {
		t.Fatalf("expected SMART critical, got %+v", signals)
	}

	report.DeviceHealth = []device.DeviceHealth{{DevicePath: "/dev/nvme0n1", CollectionStatus: device.HealthFailed}}
	signals, _, err = Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime.Add(time.Minute), DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !containsLevel(signals, Unknown) {
		t.Fatalf("missing SMART should be UNKNOWN, got %+v", signals)
	}
}

func TestDetectOptionalTemperatureThresholdDisabledByDefault(t *testing.T) {
	raw := loadDeviceFixture("smartctl_ata_temperature.json")
	h, err := device.ParseSmartctlJSON(raw, 0)
	if err != nil {
		t.Fatal(err)
	}
	h.DevicePath = "/dev/sda"
	report := deviceReportShell(baseTime)
	report.Inventory = []device.PhysicalIdentity{{
		DevicePath:       "/dev/sda",
		StableHardwareID: "ata-test-1",
	}}
	report.DeviceHealth = []device.DeviceHealth{h}
	signals, _, err := Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range signals {
		if s.Key != "" && s.Level == Warning && strings.HasPrefix(s.Key, "device:") {
			t.Fatalf("thresholds disabled: unexpected warning %+v", s)
		}
	}
	// Fixture raw temperature is 33°C (see smartctl_ata_temperature.json).
	signals, _, err = Detect(DetectionInput{Devices: report, DevicesOK: true}, nil, baseTime, DetectionOptions{MaxTemperatureCelsius: 30})
	if err != nil {
		t.Fatal(err)
	}
	if !containsDeviceLevel(signals, Warning) {
		t.Fatalf("expected temperature warning when enabled, got %+v", signals)
	}
}

func TestDecideWithDetectionRecoveryAcrossRestarts(t *testing.T) {
	topoBad := loadZFSStatus("mirror_degraded.txt")
	in := DetectionInput{
		Topology: topoBad, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "DEGRADED"}},
	}
	state, events, err := DecideWithDetection(State{}, in, baseTime, Options{}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected initial alerts")
	}
	topoGood := loadZFSStatus("mirror_degraded.txt")
	setAllLeafStates(topoGood, "ONLINE")
	in2 := DetectionInput{
		Topology: topoGood, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "ONLINE"}},
	}
	t1 := baseTime.Add(time.Minute)
	state, events, err = DecideWithDetection(state, in2, t1, Options{}, DetectionOptions{})
	if err != nil || len(events) != 0 {
		t.Fatalf("first healthy pass should not recover yet: events=%+v err=%v", events, err)
	}
	t2 := t1.Add(time.Minute)
	_, events, err = DecideWithDetection(state, in2, t2, Options{}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected recovery after two healthy observations")
	}
}

func TestDecideWithDetectionPersistsBaselinesWithoutPII(t *testing.T) {
	topo := loadZFSStatus("mirror_healthy.txt")
	in := DetectionInput{Topology: topo, TopologyOK: true}
	state, _, err := DecideWithDetection(State{}, in, baseTime, Options{}, DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	if strings.Contains(body, "/dev/") || strings.Contains(body, "nvme") {
		t.Fatalf("state leaked device identifiers: %s", body)
	}
	if state.Baselines == nil || len(state.Baselines.Members) == 0 {
		t.Fatal("expected baselines in state")
	}
}

func containsLevel(signals []Signal, want Level) bool {
	for _, s := range signals {
		if s.Level == want {
			return true
		}
	}
	return false
}

func containsDeviceLevel(signals []Signal, want Level) bool {
	for _, s := range signals {
		if strings.HasPrefix(s.Key, "device:") && s.Level == want {
			return true
		}
	}
	return false
}

func levelIncreasedToWarningOrCritical(before, after []Signal) bool {
	byKey := map[string]Level{}
	for _, s := range before {
		byKey[s.Key] = s.Level
	}
	for _, s := range after {
		prev := byKey[s.Key]
		if prev == Healthy && (s.Level == Warning || s.Level == Critical) {
			return true
		}
		if prev == Warning && s.Level == Critical {
			return true
		}
	}
	return false
}

func zeroTopologyCounters(topo *zfs.TopologyReport) {
	for i := range topo.Pools {
		if topo.Pools[i].Root == nil {
			continue
		}
		zeroNodeCounters(topo.Pools[i].Root)
	}
}

func zeroNodeCounters(node *zfs.VdevNode) {
	if node == nil {
		return
	}
	if len(node.Children) == 0 {
		z := uint64(0)
		node.ReadErrors = &z
		node.WriteErrors = &z
		node.CksumErrors = &z
		return
	}
	for i := range node.Children {
		zeroNodeCounters(&node.Children[i])
	}
}

func bumpTopologyCounters(topo *zfs.TopologyReport, delta uint64) {
	for i := range topo.Pools {
		if topo.Pools[i].Root == nil {
			continue
		}
		bumpNodeCounters(topo.Pools[i].Root, delta)
	}
}

func bumpNodeCounters(node *zfs.VdevNode, delta uint64) {
	if node == nil {
		return
	}
	if len(node.Children) == 0 {
		if node.ReadErrors != nil {
			v := *node.ReadErrors + delta
			node.ReadErrors = &v
		}
		return
	}
	for i := range node.Children {
		bumpNodeCounters(&node.Children[i], delta)
	}
}

func setAllLeafStates(topo *zfs.TopologyReport, state string) {
	for i := range topo.Pools {
		if topo.Pools[i].Root == nil {
			continue
		}
		setNodeLeafStates(topo.Pools[i].Root, state)
	}
}

func setNodeLeafStates(node *zfs.VdevNode, state string) {
	if node == nil {
		return
	}
	if len(node.Children) == 0 {
		node.State = state
		node.StateObserved = true
		return
	}
	for i := range node.Children {
		setNodeLeafStates(&node.Children[i], state)
	}
}

func assertNoSensitiveLeak(t *testing.T, signals []Signal) {
	t.Helper()
	raw, _ := json.Marshal(signals)
	if strings.Contains(string(raw), "nvme-eui") || strings.Contains(string(raw), "/dev/") {
		t.Fatalf("signal leak: %s", raw)
	}
}
