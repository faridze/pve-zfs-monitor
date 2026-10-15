package device

import (
	"context"
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestCollectorSmartCompletenessMultiDiskOneSuccess(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("pool_raidz1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		LsblkPath: func([]string) (cmdrun.Output, error) {
			return cmdrun.Output{Stdout: loadFixture("lsblk_raidz.json")}, nil
		},
		SmartctlPath: func(args []string) (cmdrun.Output, error) {
			if len(args) == 0 {
				return cmdrun.Output{}, &cmdrun.Error{Kind: cmdrun.ExitError, Path: SmartctlPath, ExitCode: 2}
			}
			dev := args[len(args)-1]
			if dev == "/dev/sda" {
				return cmdrun.Output{Stdout: loadFixture("smartctl_nvme_healthy.json")}, nil
			}
			stderr := []byte("Permission denied")
			return cmdrun.Output{Stderr: stderr}, &cmdrun.Error{
				Kind: cmdrun.ExitError, Path: SmartctlPath, ExitCode: 2, Stderr: stderr,
			}
		},
	}}
	inv, _ := ParseLsblkJSON(loadFixture("lsblk_raidz.json"))
	c := &Collector{
		Runner:          runner,
		TopologyCollect: func(context.Context) (*zfs.TopologyReport, error) { return topo, nil },
		ByIDReader:      func(string) (map[string]string, error) { return map[string]string{}, nil },
		DeviceVerifier:  stubVerifierFromInventory(inv),
	}
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.MappingCompleteness != ReportMappingComplete {
		t.Fatalf("mapping=%q", report.MappingCompleteness)
	}
	if report.SmartCompleteness == ReportSmartComplete {
		t.Fatalf("must not report smart_complete with mixed SMART outcomes: %#v", report.DeviceHealth)
	}
	if len(report.DeviceHealth) != 3 {
		t.Fatalf("expected 3 unique devices, got %d", len(report.DeviceHealth))
	}
}

func TestCollectorMappingIncompleteWhenOneMemberMissing(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("mirror_dev_partitions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		LsblkPath: func([]string) (cmdrun.Output, error) {
			return cmdrun.Output{Stdout: loadFixture("lsblk_nvme_one_disk.json")}, nil
		},
	}}
	byID, _ := ParseByIDLines(strings.Split(string(loadFixture("byid_nvme_mirror.txt")), "\n"))
	inv, _ := ParseLsblkJSON(loadFixture("lsblk_nvme_one_disk.json"))
	ApplyByIDLinks(inv, byID)
	c := &Collector{
		Runner:          runner,
		TopologyCollect: func(context.Context) (*zfs.TopologyReport, error) { return topo, nil },
		ByIDReader:      func(string) (map[string]string, error) { return byID, nil },
		DeviceVerifier:  stubVerifierFromInventory(inv),
	}
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if report.MappingCompleteness != ReportMappingIncomplete {
		t.Fatalf("mapping=%q mappings=%#v", report.MappingCompleteness, report.MemberMappings)
	}
}

func TestSmartctlExit13WithoutPermissionTextIsNotPermissionDenied(t *testing.T) {
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		SmartctlPath: func([]string) (cmdrun.Output, error) {
			return cmdrun.Output{}, &cmdrun.Error{
				Kind: cmdrun.ExitError, Path: SmartctlPath, ExitCode: 13,
			}
		},
	}}
	inv := buildTestInventory(t)
	c := &Collector{Runner: runner, DeviceVerifier: stubVerifierFromInventory(inv)}
	h := c.collectSmart(context.Background(), "/dev/nvme0n1", inv)
	if h.CollectionStatus == HealthPermissionDenied {
		t.Fatal("exit 13 without permission evidence must not be permission_denied")
	}
	if h.CollectionStatus != HealthFailed {
		t.Fatalf("status=%q", h.CollectionStatus)
	}
}
