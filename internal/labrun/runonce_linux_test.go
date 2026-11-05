//go:build linux

package labrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func labDir(t *testing.T) string {
	d := t.TempDir()
	if err := os.Chmod(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := WriteLabMarker(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func loadTopo(name string) *zfs.TopologyReport {
	data, err := os.ReadFile(filepath.Join("..", "zfs", "testdata", name))
	if err != nil {
		panic(err)
	}
	r, err := zfs.ParseStatus(data)
	if err != nil {
		panic(err)
	}
	return r
}

func deviceShell(now time.Time) *device.DeviceReport {
	passed := true
	return &device.DeviceReport{
		CollectedAt:           now.Format(time.RFC3339),
		InventoryCompleteness: device.ReportInventoryComplete,
		MappingCompleteness:   device.ReportMappingComplete,
		SmartCompleteness:     device.ReportSmartComplete,
		Inventory:             []device.PhysicalIdentity{{StableHardwareID: "disk-a"}},
		DeviceHealth: []device.DeviceHealth{{
			SmartPassed:      &passed,
			CollectionStatus: device.HealthCollected,
		}},
	}
}

func collectorsHealthy() Collectors {
	now := time.Now().UTC()
	topo := loadTopo("mirror_healthy.txt")
	return Collectors{
		ListPools: func(context.Context) ([]zfs.Pool, error) {
			return []zfs.Pool{{Name: "rpool", Health: "ONLINE"}}, nil
		},
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return topo, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return deviceShell(now), nil },
		PoolTimeout:     time.Second,
		Now:             func() time.Time { return now },
	}
}

func TestRunOnceHealthyFirstObservation(t *testing.T) {
	dir := labDir(t)
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, collectorsHealthy(), "lab")
	if err != nil {
		t.Fatal(err)
	}
	if !report.StatePersisted || report.Overall != monitor.Healthy {
		t.Fatalf("report: %+v", report)
	}
}

func TestRunOnceVerifiedCritical(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools: func(context.Context) ([]zfs.Pool, error) {
			return []zfs.Pool{{Name: "p1", Health: "FAULTED"}}, nil
		},
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
	}
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if report.VerifiedCritical == 0 && report.EventsEmittedCount == 0 {
		t.Fatalf("expected critical path: %+v", report)
	}
}

func TestRunOnceRepeatCriticalNoDuplicateAlert(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools: func(context.Context) ([]zfs.Pool, error) {
			return []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}, nil
		},
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
	}
	r1, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil || r1.EventsEmittedCount == 0 {
		t.Fatalf("first: %+v err=%v", r1, err)
	}
	r2, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if r2.EventsEmittedCount != 0 {
		t.Fatalf("duplicate alert: %+v", r2)
	}
}

func TestRunOnceRecoveryTwoSamples(t *testing.T) {
	dir := labDir(t)
	now := time.Unix(1730000000, 0).UTC()
	advance := func() func() time.Time {
		cur := now
		return func() time.Time {
			cur = cur.Add(time.Minute)
			return cur
		}
	}
	bad := Collectors{
		ListPools:       func(context.Context) ([]zfs.Pool, error) { return []zfs.Pool{{Name: "p1", Health: "FAULTED"}}, nil },
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
		Now:             advance(),
	}
	if _, err := RunOnce(context.Background(), true, dir, DeliveryNone, bad, "lab"); err != nil {
		t.Fatal(err)
	}
	good := Collectors{
		ListPools:       func(context.Context) ([]zfs.Pool, error) { return []zfs.Pool{{Name: "p1", Health: "ONLINE"}}, nil },
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
		Now:             advance(),
	}
	if _, err := RunOnce(context.Background(), true, dir, DeliveryNone, good, "lab"); err != nil {
		t.Fatal(err)
	}
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, good, "lab")
	if err != nil {
		t.Fatal(err)
	}
	hasRecovery := false
	for _, e := range report.EventsEmitted {
		if e.Kind == monitor.Recovery {
			hasRecovery = true
		}
	}
	if !hasRecovery {
		t.Fatalf("expected recovery: %+v", report.EventsEmitted)
	}
}

func TestRunOnceCollectionFailureUnknown(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools:       func(context.Context) ([]zfs.Pool, error) { return nil, os.ErrPermission },
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, os.ErrPermission },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, os.ErrPermission },
	}
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall == monitor.Healthy || len(report.CollectionFailures) == 0 {
		t.Fatalf("expected unknown/failures: %+v", report)
	}
}

func TestRunOncePendingSurvivesDeliveryNone(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools:       func(context.Context) ([]zfs.Pool, error) { return []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}, nil },
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
	}
	r1, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil || r1.PendingNotifications == 0 {
		t.Fatalf("pending: %+v err=%v", r1, err)
	}
	r2, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err != nil || r2.PendingNotifications == 0 {
		t.Fatalf("restart pending lost: %+v", r2)
	}
}

func TestRunOnceSimulatedDeliveryAck(t *testing.T) {
	dir := labDir(t)
	c := Collectors{
		ListPools:       func(context.Context) ([]zfs.Pool, error) { return []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}, nil },
		CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return nil, nil },
	}
	report, err := RunOnce(context.Background(), true, dir, DeliveryFake, c, "lab")
	if err != nil || report.SimulatedDeliveries == 0 || report.AtLeastOnceLimitation == "" {
		t.Fatalf("fake deliver: %+v err=%v", report, err)
	}
}

func TestRunOnceCorruptStateFailsClosed(t *testing.T) {
	dir := labDir(t)
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"format_version":2,"state":{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	c := collectorsHealthy()
	_, err := RunOnce(context.Background(), true, dir, DeliveryNone, c, "lab")
	if err == nil {
		t.Fatal("expected error")
	}
	after, _ := os.ReadFile(filepath.Join(dir, "state.json"))
	if string(before) != string(after) {
		t.Fatal("corrupt file modified")
	}
}

func TestRunOnceRequiresLabFlag(t *testing.T) {
	dir := labDir(t)
	_, err := RunOnce(context.Background(), false, dir, DeliveryNone, collectorsHealthy(), "lab")
	if err == nil {
		t.Fatal("expected lab flag error")
	}
}

func TestValidateLabStateDirMode(t *testing.T) {
	d := t.TempDir()
	if err := os.Chmod(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateLabStateDir(d); err == nil {
		t.Fatal("expected mode error")
	}
}

func TestRunOnceReportsCounterIncreaseAfterPersistedBaseline(t *testing.T) {
	dir := labDir(t)
	now := time.Now().UTC()
	topo := loadTopo("mirror_healthy.txt")
	baseCollectors := func(top *zfs.TopologyReport) Collectors {
		return Collectors{
			ListPools: func(context.Context) ([]zfs.Pool, error) {
				return []zfs.Pool{{Name: "rpool", Health: "ONLINE"}}, nil
			},
			CollectTopology: func(context.Context) (*zfs.TopologyReport, error) { return top, nil },
			CollectDevices:  func(context.Context) (*device.DeviceReport, error) { return deviceShell(now), nil },
			PoolTimeout:     time.Second,
			Now:             func() time.Time { return now },
		}
	}
	if _, err := RunOnce(context.Background(), true, dir, DeliveryNone, baseCollectors(topo), "lab"); err != nil {
		t.Fatal(err)
	}
	bumped := loadTopo("mirror_healthy.txt")
	bumpLeafReadErrors(bumped, 5)
	report, err := RunOnce(context.Background(), true, dir, DeliveryNone, baseCollectors(bumped), "lab")
	if err != nil {
		t.Fatal(err)
	}
	if report.Overall != monitor.Warning && report.VerifiedWarning == 0 {
		t.Fatalf("expected counter increase warning in same-detection report: %+v", report)
	}
}

func bumpLeafReadErrors(topo *zfs.TopologyReport, delta uint64) {
	if len(topo.Pools) == 0 || topo.Pools[0].Root == nil {
		return
	}
	bumpFirstLeafRead(topo.Pools[0].Root, delta)
}

func bumpFirstLeafRead(node *zfs.VdevNode, delta uint64) {
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
	bumpFirstLeafRead(&node.Children[0], delta)
}
