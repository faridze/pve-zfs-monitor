package zfs

import (
	"context"
	"errors"
	"fmt"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
)

var zpoolStatusArgs = []string{"status", "-v"}

// DefaultTopologyCollector collects topology using production zpool arguments.
var DefaultTopologyCollector = NewTopologyCollector(cmdrun.NewOSRunner(cmdrun.DefaultLimits()))

// TopologyCollector runs read-only zpool status collection.
type TopologyCollector struct {
	Runner cmdrun.Runner
}

// NewTopologyCollector returns a topology collector with the given runner.
func NewTopologyCollector(runner cmdrun.Runner) *TopologyCollector {
	return &TopologyCollector{Runner: runner}
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
	list, listErr := DefaultCollector.ListPools(ctx)
	if listErr == nil {
		report.EnrichListHealth(list)
	} else {
		for i := range report.Pools {
			report.Pools[i].ParseIssues = append(report.Pools[i].ParseIssues,
				fmt.Sprintf("zpool list unavailable: %v", listErr))
		}
	}
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
