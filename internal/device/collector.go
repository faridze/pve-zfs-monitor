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
	DeviceVerifier  DeviceVerifier
}

// DefaultCollector uses production paths and topology collection.
func DefaultCollector() *Collector {
	return &Collector{
		Runner:          cmdrun.NewOSRunner(cmdrun.DefaultLimits()),
		TopologyCollect: zfs.CollectTopology,
		ByIDReader:      func(dir string) (map[string]string, error) { return ReadByIDIndex(dir) },
		DeviceVerifier:  OSDeviceVerifier{},
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
	}
	if invErr != nil {
		report.ReportIssues = append(report.ReportIssues, invErr.Error())
		report.InventoryCompleteness = ReportInventoryFailed
		report.MemberMappings = CorrelateTopology(topo, nil)
	} else {
		report.InventoryCompleteness = inventoryCompleteness(inv)
		if inv != nil {
			report.InventoryIssues = inv.Issues
			for _, id := range inv.Devices {
				report.Inventory = append(report.Inventory, id)
			}
		}
		report.MemberMappings = CorrelateTopology(topo, inv)
	}
	report.MappingCompleteness = mappingCompleteness(report.MemberMappings)
	healthPaths := uniqueMappedPaths(report.MemberMappings)
	if len(healthPaths) == 0 {
		report.SmartCompleteness = ReportSmartNotAttempted
	} else {
		for _, path := range healthPaths {
			h := c.collectSmart(ctx, path, inv)
			report.DeviceHealth = append(report.DeviceHealth, h)
		}
		report.SmartCompleteness = smartCompleteness(report.DeviceHealth)
	}
	return report, nil
}

func inventoryCompleteness(inv *Inventory) string {
	if inv == nil || len(inv.Devices) == 0 {
		return ReportInventoryIncomplete
	}
	if len(inv.Issues) > 0 {
		return ReportInventoryIncomplete
	}
	return ReportInventoryComplete
}

func mappingCompleteness(mappings []MemberMapping) string {
	if len(mappings) == 0 {
		return ReportMappingIncomplete
	}
	for _, m := range mappings {
		if m.Outcome == MappingMapped {
			return ReportMappingComplete
		}
	}
	return ReportMappingIncomplete
}

func smartCompleteness(health []DeviceHealth) string {
	if len(health) == 0 {
		return ReportSmartNotAttempted
	}
	for _, h := range health {
		switch h.CollectionStatus {
		case HealthCollected, HealthPartial, HealthConflict:
			return ReportSmartComplete
		}
	}
	return ReportSmartIncomplete
}

func (c *Collector) collectInventory(ctx context.Context) (*Inventory, error) {
	out, err := c.Runner.Run(ctx, LsblkPath, LsblkArgs)
	if err != nil {
		return nil, mapInventoryError(err)
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

func (c *Collector) collectSmart(ctx context.Context, devicePath string, inv *Inventory) DeviceHealth {
	h := DeviceHealth{DevicePath: devicePath, CollectionStatus: HealthNotAttempted}
	verifier := c.DeviceVerifier
	if verifier == nil {
		verifier = StubDeviceVerifier{}
	}
	target, err := ResolveSMARTTarget(devicePath, inv, verifier)
	if err != nil {
		h.CollectionStatus = HealthFailed
		h.CollectionDetail = err.Error()
		return h
	}
	h.DevicePath = target
	out, err := c.Runner.Run(ctx, SmartctlPath, SmartctlHealthArgs(target))
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
				if smartctlPermissionDenied(runErr, out.Stderr) {
					h.CollectionStatus = HealthPermissionDenied
					h.CollectionDetail = runErr.Error()
					return h
				}
				if len(runErr.Stdout) > 0 {
					parsed, perr := ParseSmartctlJSON(runErr.Stdout, runErr.ExitCode)
					if perr == nil {
						parsed.DevicePath = target
						return parsed
					}
				}
				if SmartctlExitIndicatesOperationalError(runErr.ExitCode) {
					h.CollectionStatus = HealthFailed
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
	parsed.DevicePath = target
	return parsed
}

func smartctlPermissionDenied(runErr *cmdrun.Error, outStderr []byte) bool {
	if runErr == nil {
		return false
	}
	if runErr.ExitCode == 13 {
		return true
	}
	stderr := runErr.Stderr
	if len(stderr) == 0 {
		stderr = outStderr
	}
	s := strings.ToLower(string(stderr))
	return strings.Contains(s, "permission denied") ||
		strings.Contains(s, "administrator privileges") ||
		strings.Contains(s, "root privileges")
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
