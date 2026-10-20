package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

// DetectionInput bundles read-only collector output for offline evaluation.
type DetectionInput struct {
	Pools      []zfs.Pool
	PoolsOK    bool
	Topology   *zfs.TopologyReport
	TopologyOK bool
	Devices    *device.DeviceReport
	DevicesOK  bool
}

// DetectionOptions configures optional, conservative thresholds (0 disables).
type DetectionOptions struct {
	MaxTemperatureCelsius int
	MaxPercentageUsed     int
	// MaxReportAge bounds DeviceReport.CollectedAt relative to evaluation time (Detect observedAt).
	MaxReportAge time.Duration
}

// Detect evaluates topology and SMART/device evidence into privacy-safe signals
// and updated counter baselines. It never invokes external tools.
func Detect(in DetectionInput, prev *Baselines, observedAt time.Time, opts DetectionOptions) ([]Signal, *Baselines, error) {
	if observedAt.IsZero() {
		return nil, nil, fmt.Errorf("invalid observation time")
	}
	opts = normalizeDetectionOptions(opts)
	next := cloneBaselines(prev)
	var out []Signal

	poolSignals, err := PoolSignals(in.Pools, observedAt, in.PoolsOK)
	if err != nil {
		return nil, nil, err
	}
	out = append(out, poolSignals...)

	topoSignals, memberBase, memberClear, err := topologySignals(in.Topology, in.TopologyOK, in.Devices, prev, observedAt)
	if err != nil {
		return nil, nil, err
	}
	out = append(out, topoSignals...)
	mergeMemberBaselines(next, memberBase)
	clearMemberBaselines(next, memberClear)

	deviceAt, deviceTimeOK, err := resolveDeviceObservationTime(in.Devices, in.DevicesOK, observedAt, opts.MaxReportAge)
	if err != nil {
		return nil, nil, err
	}
	if in.DevicesOK && in.Devices != nil && !deviceTimeOK {
		return nil, nil, fmt.Errorf("device report freshness could not be verified")
	}
	deviceObservedAt := observedAt
	if deviceTimeOK {
		deviceObservedAt = deviceAt
	}

	deviceSignals, deviceBase, deviceClear, err := deviceSignals(in.Devices, in.DevicesOK, prev, deviceObservedAt, opts)
	if err != nil {
		return nil, nil, err
	}
	out = append(out, deviceSignals...)
	mergeDeviceBaselines(next, deviceBase)
	clearDeviceBaselines(next, deviceClear)

	if err := validateBaselines(next); err != nil {
		return nil, nil, err
	}
	if len(out) > maxSignals {
		return nil, nil, fmt.Errorf("too many signals")
	}
	return out, next, nil
}

// DecideWithDetection runs Detect then Decide and copies baselines into state.
func DecideWithDetection(previous State, in DetectionInput, now time.Time, opts Options, detectOpts DetectionOptions) (State, []Event, error) {
	readings, baselines, err := Detect(in, previous.Baselines, now, detectOpts)
	if err != nil {
		return State{}, nil, err
	}
	next, events, err := Decide(previous, readings, now, opts)
	if err != nil {
		return State{}, nil, err
	}
	next.Baselines = baselines
	return next, events, nil
}

func cloneBaselines(prev *Baselines) *Baselines {
	if prev == nil {
		return &Baselines{Members: make(map[string]MemberCounters), Devices: make(map[string]DeviceCounters)}
	}
	out := &Baselines{
		Members: make(map[string]MemberCounters, len(prev.Members)),
		Devices: make(map[string]DeviceCounters, len(prev.Devices)),
	}
	for k, v := range prev.Members {
		out.Members[k] = v
	}
	for k, v := range prev.Devices {
		out.Devices[k] = v
	}
	return out
}

func mergeMemberBaselines(dst *Baselines, src map[string]MemberCounters) {
	for k, v := range src {
		dst.Members[k] = v
	}
}

func mergeDeviceBaselines(dst *Baselines, src map[string]DeviceCounters) {
	for k, v := range src {
		dst.Devices[k] = v
	}
}

func clearMemberBaselines(dst *Baselines, keys []string) {
	for _, k := range keys {
		delete(dst.Members, k)
	}
}

func clearDeviceBaselines(dst *Baselines, keys []string) {
	for _, k := range keys {
		delete(dst.Devices, k)
	}
}

func memberIdentity(pool, member string, mapping *device.MemberMapping) string {
	if mapping != nil && mapping.Physical != nil && mapping.Physical.StableHardwareID != "" {
		return "hw:" + mapping.Physical.StableHardwareID
	}
	return "zfs:" + pool + "\x00" + member
}

func deviceIdentity(phys *device.PhysicalIdentity) (string, error) {
	if phys == nil {
		return "", fmt.Errorf("missing physical identity")
	}
	if phys.StableHardwareID != "" {
		return "hw:" + phys.StableHardwareID, nil
	}
	if phys.RuntimeLocator != "" {
		return "loc:" + phys.RuntimeLocator, nil
	}
	return "", fmt.Errorf("no stable device identity")
}

func zfsMemberLevel(state string, verified bool) Level {
	if !verified {
		return Unknown
	}
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "ONLINE", "INUSE":
		return Healthy
	case "DEGRADED":
		return Warning
	case "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "UNAVAILIBLE":
		return Critical
	default:
		return Unknown
	}
}
