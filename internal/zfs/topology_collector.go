package zfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
)

var zpoolStatusArgs = []string{"status", "-v"}

// DefaultTopologyCollector collects topology using production zpool arguments.
var DefaultTopologyCollector = NewTopologyCollectorWithList(
	cmdrun.NewOSRunner(cmdrun.DefaultLimits()),
	ListPools,
)

// TopologyCollector runs read-only zpool status collection.
type TopologyCollector struct {
	Runner    cmdrun.Runner
	ListPools func(context.Context) ([]Pool, error)
}

// NewTopologyCollector returns a collector with the production list-health hook.
func NewTopologyCollector(runner cmdrun.Runner) *TopologyCollector {
	return NewTopologyCollectorWithList(runner, ListPools)
}

// NewTopologyCollectorWithList allows injecting or disabling zpool list comparison.
// When listFn is nil, list health remains unverified.
func NewTopologyCollectorWithList(runner cmdrun.Runner, listFn func(context.Context) ([]Pool, error)) *TopologyCollector {
	return &TopologyCollector{Runner: runner, ListPools: listFn}
}

// CollectTopology gathers pool topology and optional list-health consistency.
func CollectTopology(ctx context.Context) (*TopologyReport, error) {
	return DefaultTopologyCollector.Collect(ctx)
}

// Collect executes zpool status -v and parses topology.
func (c *TopologyCollector) Collect(ctx context.Context) (*TopologyReport, error) {
	out, err := c.Runner.Run(ctx, ZpoolPath, zpoolStatusArgs)
	if err != nil {
		return nil, mapStatusError(err)
	}
	report, err := ParseStatus(out.Stdout)
	if err != nil {
		return nil, err
	}
	if c.ListPools == nil {
		report.MarkListHealthUnverified("zpool list comparison not configured")
		return report, nil
	}
	list, listErr := c.ListPools(ctx)
	if listErr != nil {
		report.MarkListHealthUnverified(fmt.Sprintf("zpool list unavailable: %v", listErr))
		return report, nil
	}
	report.EnrichListHealth(list)
	return report, nil
}

func mapStatusError(err error) error {
	var runErr *cmdrun.Error
	if !errors.As(err, &runErr) {
		return fmt.Errorf("zpool status failed: %w", err)
	}
	switch runErr.Kind {
	case cmdrun.NotAvailable:
		return fmt.Errorf("%s not available", runErr.Path)
	case cmdrun.Timeout:
		return fmt.Errorf("zpool status timed out: %w", runErr.Cause)
	case cmdrun.Truncated:
		return fmt.Errorf("zpool status output truncated")
	case cmdrun.ReadError:
		return fmt.Errorf("zpool status failed: %w", runErr)
	case cmdrun.ExitError:
		return fmt.Errorf("zpool status failed: %w", runErr)
	default:
		return fmt.Errorf("zpool status failed: %w", err)
	}
}
