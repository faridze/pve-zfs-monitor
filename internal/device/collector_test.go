package device

import (
	"context"
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

type stubRunner struct {
	handlers map[string]func([]string) (cmdrun.Output, error)
}

func (s *stubRunner) Run(ctx context.Context, path string, args []string) (cmdrun.Output, error) {
	if h, ok := s.handlers[path]; ok {
		return h(args)
	}
	return cmdrun.Output{}, &cmdrun.Error{Kind: cmdrun.NotAvailable, Path: path}
}

func TestCollectorEndToEndSynthetic(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("mirror_dev_partitions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		LsblkPath: func(args []string) (cmdrun.Output, error) {
			return cmdrun.Output{Stdout: loadFixture("lsblk_nvme_mirror.json")}, nil
		},
		SmartctlPath: func(args []string) (cmdrun.Output, error) {
			return cmdrun.Output{Stdout: loadFixture("smartctl_nvme_healthy.json")}, nil
		},
	}}
	byID, _ := ParseByIDLines(strings.Split(string(loadFixture("byid_nvme_mirror.txt")), "\n"))
	c := &Collector{
		Runner:          runner,
		TopologyCollect: func(context.Context) (*zfs.TopologyReport, error) { return topo, nil },
		ByIDReader:      func(string) (map[string]string, error) { return byID, nil },
	}
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MemberMappings) < 2 {
		t.Fatalf("mappings: %#v", report.MemberMappings)
	}
	if len(report.DeviceHealth) == 0 {
		t.Fatal("expected device health entries")
	}
	for _, h := range report.DeviceHealth {
		if h.CollectionStatus == HealthCollected && h.SmartPassed == nil {
			t.Fatalf("collected health without smart_passed: %#v", h)
		}
	}
}

func TestCollectorMissingSmartctlStillReturnsReport(t *testing.T) {
	topo, _ := zfs.ParseStatus(loadZFSFixture("pool_single_disk.txt"))
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		LsblkPath: func([]string) (cmdrun.Output, error) {
			return cmdrun.Output{Stdout: loadFixture("lsblk_single_disk.json")}, nil
		},
	}}
	c := &Collector{
		Runner:          runner,
		TopologyCollect: func(context.Context) (*zfs.TopologyReport, error) { return topo, nil },
		ByIDReader:      func(string) (map[string]string, error) { return map[string]string{}, nil },
	}
	report, err := c.Collect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(report.DeviceHealth) == 0 {
		t.Fatal("expected health attempt")
	}
	if report.DeviceHealth[0].CollectionStatus != HealthMissingTool {
		t.Fatalf("status=%q", report.DeviceHealth[0].CollectionStatus)
	}
}

func TestCollectorSmartctlNonzeroExitWithJSON(t *testing.T) {
	runner := &stubRunner{handlers: map[string]func([]string) (cmdrun.Output, error){
		SmartctlPath: func(args []string) (cmdrun.Output, error) {
			return cmdrun.Output{}, &cmdrun.Error{
				Kind:     cmdrun.ExitError,
				Path:     SmartctlPath,
				ExitCode: 64,
				Stdout:   loadFixture("smartctl_critical_exit.json"),
			}
		},
	}}
	c := &Collector{Runner: runner}
	h := c.collectSmart(context.Background(), "/dev/nvme0n1")
	if h.CollectionStatus != HealthCollected {
		t.Fatalf("status=%q", h.CollectionStatus)
	}
	if h.SmartPassed == nil || *h.SmartPassed {
		t.Fatalf("smart_passed=%v", h.SmartPassed)
	}
}
