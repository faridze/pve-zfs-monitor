package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

const defaultMaxReportAge = 10 * time.Minute

func normalizeDetectionOptions(o DetectionOptions) DetectionOptions {
	if o.MaxReportAge <= 0 {
		o.MaxReportAge = defaultMaxReportAge
	}
	return o
}

// resolveDeviceObservationTime returns the collector timestamp for device evidence.
// evalAt is when detection/decision runs; it must not be substituted for collection time.
func resolveDeviceObservationTime(report *device.DeviceReport, devicesOK bool, evalAt time.Time, maxAge time.Duration) (time.Time, bool, error) {
	if !devicesOK {
		return time.Time{}, false, nil
	}
	if report == nil {
		return time.Time{}, false, nil
	}
	if strings.TrimSpace(report.CollectedAt) == "" {
		return time.Time{}, false, fmt.Errorf("device report missing collected_at")
	}
	collected, err := time.Parse(time.RFC3339, report.CollectedAt)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("device report collected_at invalid")
	}
	if collected.IsZero() {
		return time.Time{}, false, fmt.Errorf("device report collected_at invalid")
	}
	if collected.After(evalAt) {
		return time.Time{}, false, fmt.Errorf("device report collected_at in the future")
	}
	if maxAge > 0 && evalAt.Sub(collected) > maxAge {
		return time.Time{}, false, fmt.Errorf("device report stale")
	}
	return collected.UTC(), true, nil
}

func deviceCoverageFromReport(report *device.DeviceReport) (Level, bool) {
	if report == nil {
		return Unknown, false
	}
	if len(report.Inventory) == 0 && len(report.DeviceHealth) == 0 &&
		report.SmartCompleteness == "" && report.InventoryCompleteness == "" {
		return Unknown, false
	}
	if len(report.ReportIssues) > 0 {
		return Unknown, false
	}
	switch report.InventoryCompleteness {
	case device.ReportInventoryComplete:
	case device.ReportInventoryIncomplete, device.ReportInventoryFailed:
		return Unknown, false
	default:
		return Unknown, false
	}
	switch report.MappingCompleteness {
	case device.ReportMappingComplete:
	case device.ReportMappingIncomplete:
		return Unknown, false
	default:
		return Unknown, false
	}
	switch report.SmartCompleteness {
	case device.ReportSmartComplete:
		return Healthy, true
	case device.ReportSmartIncomplete, device.ReportSmartNotAttempted:
		return Unknown, false
	default:
		return Unknown, false
	}
}

func hasUsableSMARTEvidence(h device.DeviceHealth) bool {
	if h.SmartPassed != nil {
		return true
	}
	if h.CriticalWarning != nil {
		return true
	}
	if strings.TrimSpace(h.SmartStatusString) != "" {
		return true
	}
	if numericInt64(h.MediaErrors) != nil || numericInt64(h.DataIntegrityErrors) != nil ||
		numericInt64(h.ErrorLogEntries) != nil {
		return true
	}
	if h.PercentageUsed != nil && h.PercentageUsed.Value != nil {
		return true
	}
	if h.AvailableSpare != nil && h.AvailableSpare.Value != nil {
		return true
	}
	// Temperature alone is ancillary; it does not prove overall SMART health.
	return false
}

func zfsVerifiedCriticalMember(leaf zfs.VdevNode) bool {
	if !leaf.StateObserved {
		return false
	}
	switch strings.ToUpper(strings.TrimSpace(leaf.State)) {
	case "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "UNAVAILIBLE":
		return true
	default:
		return false
	}
}

func logicalMemberIdentity(pool, member string) string {
	return "zfs:" + pool + "\x00" + member
}

func topologyCollectionVerified(topo *zfs.TopologyReport) bool {
	if topo == nil {
		return false
	}
	if len(topo.ReportIssues) > 0 {
		return false
	}
	switch topo.ListComparisonStatus {
	case zfs.ReportListComparisonComplete:
	case zfs.ReportListComparisonUnverified, zfs.ReportListComparisonIncomplete,
		zfs.ReportListComparisonInconsistent:
		return false
	default:
		// Not enriched or unknown status: require per-pool list health below.
	}
	if len(topo.Pools) == 0 {
		return false
	}
	for _, pool := range topo.Pools {
		if pool.Root == nil {
			return false
		}
		if len(pool.ParseIssues) > 0 {
			return false
		}
		switch pool.ListHealthStatus {
		case zfs.ListHealthVerifiedConsistent:
		case zfs.ListHealthVerifiedInconsistent, zfs.ListHealthUnverified, "":
			return false
		default:
			return false
		}
	}
	if topo.ListComparisonStatus != "" && topo.ListComparisonStatus != zfs.ReportListComparisonComplete {
		return false
	}
	return true
}

// zfsStateSeverity preserves verified ZFS member state even when physical mapping is incomplete.
func zfsStateSeverity(leaf zfs.VdevNode) (Level, bool) {
	if !leaf.StateObserved {
		return Unknown, false
	}
	level := zfsMemberLevel(leaf.State, true)
	if level == Critical || level == Warning {
		return level, true
	}
	return level, level == Healthy
}
