package labrun

import (
	"context"
	"fmt"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

// Collectors provides read-only telemetry for one lab cycle.
type Collectors struct {
	ListPools       func(context.Context) ([]zfs.Pool, error)
	CollectTopology func(context.Context) (*zfs.TopologyReport, error)
	CollectDevices  func(context.Context) (*device.DeviceReport, error)
	PoolTimeout     time.Duration
	DevicesTimeout  time.Duration
	// Now overrides wall clock for tests; nil uses time.Now().UTC().
	Now func() time.Time
}

// BuildDetectionInput runs collectors and constructs monitor input with fail-closed flags.
func BuildDetectionInput(ctx context.Context, c Collectors) (monitor.DetectionInput, []string, error) {
	if c.PoolTimeout <= 0 {
		c.PoolTimeout = 10 * time.Second
	}
	if c.DevicesTimeout <= 0 {
		c.DevicesTimeout = c.PoolTimeout
	}
	var failures []string
	poolCtx, cancel := context.WithTimeout(ctx, c.PoolTimeout)
	pools, poolErr := c.ListPools(poolCtx)
	cancel()
	poolsOK := poolErr == nil
	if poolErr != nil {
		failures = append(failures, "pools: "+poolErr.Error())
	}

	topoCtx, cancel := context.WithTimeout(ctx, c.PoolTimeout)
	topo, topoErr := c.CollectTopology(topoCtx)
	cancel()
	topoOK := topoErr == nil
	if topoErr != nil {
		failures = append(failures, "topology: "+topoErr.Error())
	} else if topo != nil && poolsOK {
		enrichTopologyHealth(topo, pools)
	}

	devCtx, cancel := context.WithTimeout(ctx, c.DevicesTimeout)
	devReport, devErr := c.CollectDevices(devCtx)
	cancel()
	devicesOK := devErr == nil
	if devErr != nil {
		failures = append(failures, "devices: "+devErr.Error())
	}

	in := monitor.DetectionInput{
		Pools: pools, PoolsOK: poolsOK,
		Topology: topo, TopologyOK: topoOK,
		Devices: devReport, DevicesOK: devicesOK,
	}
	return in, failures, nil
}

func enrichTopologyHealth(topo *zfs.TopologyReport, pools []zfs.Pool) {
	if topo == nil {
		return
	}
	healthByName := make(map[string]string, len(pools))
	for _, p := range pools {
		healthByName[p.Name] = p.Health
	}
	var list []zfs.Pool
	for name, health := range healthByName {
		list = append(list, zfs.Pool{Name: name, Health: health})
	}
	topo.EnrichListHealth(list)
}

// DefaultMonitorOptions returns conservative lab decision options.
func DefaultMonitorOptions() (monitor.Options, monitor.DetectionOptions) {
	return monitor.Options{
			MaxAge:           time.Hour,
			ReminderInterval: 24 * time.Hour,
			RecoverySamples:  2,
		}, monitor.DetectionOptions{
			MaxReportAge: 10 * time.Minute,
		}
}

// SignalSummary runs Detect for reporting only (after persistence).
func SignalSummary(in monitor.DetectionInput, baselines *monitor.Baselines, now time.Time, detect monitor.DetectionOptions) ([]monitor.Signal, error) {
	signals, _, err := monitor.Detect(in, baselines, now, detect)
	if err != nil {
		return nil, fmt.Errorf("signal summary: %w", err)
	}
	return signals, nil
}

func evalNow(c Collectors) time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}
