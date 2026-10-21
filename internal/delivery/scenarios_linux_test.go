//go:build linux

package delivery

import (
	"os"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/observium"
	"github.com/faridze/pve-zfs-monitor/internal/outbox"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func loadTopo(name string) *zfs.TopologyReport {
	data, err := os.ReadFile("../zfs/testdata/" + name)
	if err != nil {
		panic(err)
	}
	r, err := zfs.ParseStatus(data)
	if err != nil {
		panic(err)
	}
	return r
}

func TestScenarioMirrorDegradedObserviumNotFullyHealthy(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	topo := loadTopo("mirror_degraded.txt")
	topo.EnrichListHealth([]zfs.Pool{{Name: "rpool", Health: "DEGRADED"}})
	report := deviceReport(now)
	in := monitor.DetectionInput{
		Topology: topo, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "DEGRADED"}},
		Devices: report, DevicesOK: true,
	}
	signals, _, err := monitor.Detect(in, nil, now, monitor.DetectionOptions{MaxReportAge: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	sum, err := observium.BuildSummary(observium.ExportInput{
		Signals: signals, CollectedAt: now, EvaluatedAt: now,
		MaxSignalAge: time.Hour, SmartCompleteness: observium.SmartComplete,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum.ReportFullyHealthy || sum.VerifiedCriticalCount == 0 {
		t.Fatalf("degraded mirror: %+v", sum)
	}
	_, err = observium.RenderAppSection(sum)
	if err != nil {
		t.Fatal(err)
	}
}

func TestScenarioRecoveryTwoSamples(t *testing.T) {
	dir := privateDir(t)
	c := Coordinator{Store: Store{Directory: dir}}
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	bad := monitor.DetectionInput{PoolsOK: true, Pools: []zfs.Pool{{Name: "p1", Health: "FAULTED"}}}
	_, _, err := c.AdvanceDetection(bad, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	good := monitor.DetectionInput{PoolsOK: true, Pools: []zfs.Pool{{Name: "p1", Health: "ONLINE"}}}
	_, _, err = c.AdvanceDetection(good, now.Add(time.Minute), monitor.Options{MaxAge: time.Hour, RecoverySamples: 2}, monitor.DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, events, err := c.AdvanceDetection(good, now.Add(2*time.Minute), monitor.Options{MaxAge: time.Hour, RecoverySamples: 2}, monitor.DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	hasRecovery := false
	for _, e := range events {
		if e.Kind == monitor.Recovery {
			hasRecovery = true
		}
	}
	if !hasRecovery {
		t.Fatalf("expected recovery event: %+v", events)
	}
}

func TestScenarioTransportFailThenRetry(t *testing.T) {
	dir := privateDir(t)
	c := Coordinator{Store: Store{Directory: dir}}
	now := time.Unix(1730000000, 0).UTC()
	in := monitor.DetectionInput{PoolsOK: true, Pools: []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}}
	_, _, err := c.AdvanceDetection(in, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	fail := &outbox.FakeTransport{Err: os.ErrClosed}
	if _, err := c.DeliverNotifications("lab", fail, now); err != nil {
		t.Fatal(err)
	}
	ok := &outbox.FakeTransport{}
	n, err := c.DeliverNotifications("lab", ok, now.Add(31*time.Second))
	if err != nil || n == 0 {
		t.Fatalf("retry deliver n=%d err=%v", n, err)
	}
}

func deviceReport(now time.Time) *device.DeviceReport {
	return &device.DeviceReport{
		CollectedAt:           now.Format(time.RFC3339),
		InventoryCompleteness: device.ReportInventoryComplete,
		MappingCompleteness:   device.ReportMappingComplete,
		SmartCompleteness:     device.ReportSmartComplete,
	}
}

func exportFromDetection(t *testing.T, in monitor.DetectionInput, detect monitor.DetectionOptions, now time.Time, smart observium.SmartCompleteness, pending int) observium.Summary {
	signals, _, err := monitor.Detect(in, nil, now, detect)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := observium.BuildSummary(observium.ExportInput{
		Signals: signals, CollectedAt: now, EvaluatedAt: now,
		MaxSignalAge: time.Hour, SmartCompleteness: smart, EventsPending: pending,
	})
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestScenarioHealthyMirrorExport(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	topo := loadTopo("mirror_healthy.txt")
	topo.EnrichListHealth([]zfs.Pool{{Name: "rpool", Health: "ONLINE"}})
	in := monitor.DetectionInput{
		Topology: topo, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "ONLINE"}},
		Devices: deviceReport(now), DevicesOK: true,
	}
	sum := exportFromDetection(t, in, monitor.DetectionOptions{MaxReportAge: time.Hour}, now, observium.SmartComplete, 0)
	if !sum.ReportFullyHealthy || sum.WorstLevel != monitor.Healthy {
		t.Fatalf("healthy mirror: %+v", sum)
	}
	section, err := observium.RenderAppSection(sum)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := observium.ParseAppSection(section); err != nil {
		t.Fatal(err)
	}
}

func TestScenarioMemberFaultedPoolOnlineExport(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	topo := loadTopo("mirror_faulted_counters.txt")
	topo.EnrichListHealth([]zfs.Pool{{Name: "rpool", Health: "ONLINE"}})
	in := monitor.DetectionInput{
		Topology: topo, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "ONLINE"}},
		Devices: deviceReport(now), DevicesOK: true,
	}
	sum := exportFromDetection(t, in, monitor.DetectionOptions{MaxReportAge: time.Hour}, now, observium.SmartComplete, 0)
	if sum.ReportFullyHealthy || sum.VerifiedCriticalCount == 0 {
		t.Fatalf("faulted member: %+v", sum)
	}
}

func TestScenarioStaleSmartNotFullyHealthy(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	topo := loadTopo("mirror_healthy.txt")
	topo.EnrichListHealth([]zfs.Pool{{Name: "rpool", Health: "ONLINE"}})
	in := monitor.DetectionInput{
		Topology: topo, TopologyOK: true,
		PoolsOK: true, Pools: []zfs.Pool{{Name: "rpool", Health: "ONLINE"}},
		DevicesOK: false,
	}
	sum := exportFromDetection(t, in, monitor.DetectionOptions{MaxReportAge: time.Hour}, now, observium.SmartUnknownReport, 0)
	if sum.ReportFullyHealthy {
		t.Fatalf("missing smart: %+v", sum)
	}
}

func TestScenarioRestartBeforeDelivery(t *testing.T) {
	dir := privateDir(t)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	c := Coordinator{Store: Store{Directory: dir}}
	in := monitor.DetectionInput{PoolsOK: true, Pools: []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}}
	_, _, err := c.AdvanceDetection(in, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	c2 := Coordinator{Store: Store{Directory: dir}}
	n, err := c2.PendingNotifications()
	if err != nil || n == 0 {
		t.Fatalf("pending after restart before deliver: %d err=%v", n, err)
	}
}

func TestScenarioMultipleFailuresSingleAdvance(t *testing.T) {
	dir := privateDir(t)
	c := Coordinator{Store: Store{Directory: dir}}
	now := time.Unix(1730000100, 0).UTC()
	in := monitor.DetectionInput{
		PoolsOK: true,
		Pools: []zfs.Pool{
			{Name: "p1", Health: "DEGRADED"},
			{Name: "p2", Health: "FAULTED"},
		},
	}
	_, events, err := c.AdvanceDetection(in, now, monitor.Options{MaxAge: time.Hour}, monitor.DetectionOptions{})
	if err != nil || len(events) < 2 {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	fake := &outbox.FakeTransport{}
	n, err := c.DeliverNotifications("lab", fake, now)
	if err != nil || n < 2 {
		t.Fatalf("deliver n=%d err=%v", n, err)
	}
}

func TestScenarioInvalidObserviumSectionRejected(t *testing.T) {
	_, err := observium.ParseAppSection("<<<pve_zfs_monitor>>>\nschema_version=1\n")
	if err == nil {
		t.Fatal("expected schema rejection")
	}
}
