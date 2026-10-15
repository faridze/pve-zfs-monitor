package device

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

// Collector performs read-only device inventory, correlation and SMART collection.
type Collector struct {
	Runner          cmdrun.Runner
	TopologyCollect func(context.Context) (*zfs.TopologyReport, error)
	ByIDReader      func(string) (map[string]string, error)
}

// DefaultCollector uses production paths and topology collection.
func DefaultCollector() *Collector {
	return &Collector{
		Runner:          cmdrun.NewOSRunner(cmdrun.DefaultLimits()),
		TopologyCollect: zfs.CollectTopology,
		ByIDReader:      func(dir string) (map[string]string, error) { return ReadByIDIndex(dir) },
	}
}

// Collect builds a device report from topology and host evidence.
func (c *Collector) Collect(ctx context.Context) (*DeviceReport, error) {
	topo, err := c.TopologyCollect(ctx)
	if err != nil {
		return nil, fmt.Errorf("topology: %w", err)
	}
	inv, invErr := c.collectInventory(ctx)
	report := &DeviceReport{
		CollectedAt:    time.Now().UTC().Format(time.RFC3339),
		TopologySource: "zpool status -v",
		MemberMappings: CorrelateTopology(topo, inv),
	}
	if inv != nil {
		report.InventoryIssues = inv.Issues
		for _, id := range inv.Devices {
			report.Inventory = append(report.Inventory, id)
		}
	}
	if invErr != nil {
		report.ReportIssues = append(report.ReportIssues, invErr.Error())
	}
	healthPaths := uniqueMappedPaths(report.MemberMappings)
	for _, path := range healthPaths {
		h := c.collectSmart(ctx, path)
		report.DeviceHealth = append(report.DeviceHealth, h)
	}
	return report, nil
}

func (c *Collector) collectInventory(ctx context.Context) (*Inventory, error) {
	out, err := c.Runner.Run(ctx, LsblkPath, LsblkArgs)
	if err != nil {
		return &Inventory{Devices: make(map[string]PhysicalIdentity)}, mapInventoryError(err)
	}
	inv, err := ParseLsblkJSON(out.Stdout)
	if err != nil {
		return nil, err
	}
	if c.ByIDReader != nil {
		byID, err := c.ByIDReader(ByIDDir)
		if err != nil {
			inv.Issues = append(inv.Issues, fmt.Sprintf("by-id unreadable: %v", err))
		} else {
			ApplyByIDLinks(inv, byID)
		}
	}
	return inv, nil
}

func mapInventoryError(err error) error {
	var runErr *cmdrun.Error
	if errors.As(err, &runErr) && runErr.Kind == cmdrun.NotAvailable {
		return fmt.Errorf("lsblk not available")
	}
	return fmt.Errorf("inventory: %w", err)
}

func (c *Collector) collectSmart(ctx context.Context, devicePath string) DeviceHealth {
	h := DeviceHealth{DevicePath: devicePath, CollectionStatus: HealthNotAttempted}
	if err := ValidateDevicePath(devicePath); err != nil {
		h.CollectionStatus = HealthFailed
		h.CollectionDetail = err.Error()
		return h
	}
	out, err := c.Runner.Run(ctx, SmartctlPath, SmartctlHealthArgs(devicePath))
	if err != nil {
		var runErr *cmdrun.Error
		if errors.As(err, &runErr) {
			switch runErr.Kind {
			case cmdrun.NotAvailable:
				h.CollectionStatus = HealthMissingTool
				h.CollectionDetail = "smartctl not available"
				return h
			case cmdrun.Timeout:
				h.CollectionStatus = HealthTimeout
				h.CollectionDetail = runErr.Error()
				return h
			case cmdrun.ExitError:
				if len(runErr.Stdout) > 0 {
					parsed, perr := ParseSmartctlJSON(runErr.Stdout, runErr.ExitCode)
					if perr == nil {
						parsed.DevicePath = devicePath
						parsed.CollectionStatus = HealthCollected
						if SmartctlExitIndicatesFailure(runErr.ExitCode) && parsed.SmartPassed == nil {
							f := false
							parsed.SmartPassed = &f
						}
						return parsed
					}
				}
				if runErr.ExitCode == 13 || strings.Contains(string(runErr.Stderr), "Permission denied") {
					h.CollectionStatus = HealthPermissionDenied
					h.CollectionDetail = runErr.Error()
					return h
				}
				h.CollectionStatus = HealthFailed
				h.CollectionDetail = runErr.Error()
				return h
			default:
				h.CollectionStatus = HealthFailed
				h.CollectionDetail = runErr.Error()
				return h
			}
		}
		h.CollectionStatus = HealthFailed
		h.CollectionDetail = err.Error()
		return h
	}
	parsed, err := ParseSmartctlJSON(out.Stdout, 0)
	if err != nil {
		h.CollectionStatus = HealthFailed
		h.CollectionDetail = err.Error()
		return h
	}
	parsed.DevicePath = devicePath
	parsed.CollectionStatus = HealthCollected
	return parsed
}

func uniqueMappedPaths(mappings []MemberMapping) []string {
	seen := make(map[string]bool)
	var out []string
	for _, m := range mappings {
		if m.Outcome != MappingMapped || m.DevicePath == "" {
			continue
		}
		if seen[m.DevicePath] {
			continue
		}
		seen[m.DevicePath] = true
		out = append(out, m.DevicePath)
	}
	return out
}
