// Package zfs contains read-only ZFS health collection and assessment.
package zfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
)

// ZpoolPath is the fixed read-only zpool binary location on Proxmox hosts.
const ZpoolPath = "/usr/sbin/zpool"

var zpoolListHealthArgs = []string{"list", "-H", "-o", "name,health"}

// DefaultCollector is the production pool list collector.
var DefaultCollector = NewPoolCollector(cmdrun.NewOSRunner(cmdrun.DefaultLimits()))

type Pool struct {
	Name   string `json:"name"`
	Health string `json:"health"`
}

type Result int

const (
	Healthy   Result = 0
	Unhealthy Result = 2
	Unknown   Result = 3
)

func (r Result) Label() string {
	switch r {
	case Healthy:
		return "healthy"
	case Unhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// PoolCollector lists pool health using a fixed read-only zpool invocation.
type PoolCollector struct {
	Runner cmdrun.Runner
}

// NewPoolCollector returns a collector that uses runner for command execution.
// Production callers must not vary zpool arguments; only Runner is injectable for tests.
func NewPoolCollector(runner cmdrun.Runner) *PoolCollector {
	return &PoolCollector{Runner: runner}
}

// Assess never treats an unfamiliar health value as healthy.
func Assess(pools []Pool) Result {
	if len(pools) == 0 {
		return Unknown
	}
	result := Healthy
	for _, p := range pools {
		switch p.Health {
		case "ONLINE":
		case "DEGRADED", "FAULTED", "OFFLINE", "UNAVAIL", "REMOVED", "SUSPENDED":
			result = Unhealthy
		default:
			if result != Unhealthy {
				result = Unknown
			}
		}
	}
	return result
}

// ParseList handles the tab-separated output of "zpool list -H -o name,health".
func ParseList(output []byte) ([]Pool, error) {
	scan := bufio.NewScanner(strings.NewReader(string(output)))
	scan.Buffer(make([]byte, 4096), 1024*1024)
	var pools []Pool
	seen := make(map[string]bool)
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			return nil, fmt.Errorf("malformed zpool output")
		}
		name, health := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		if name == "" || health == "" || seen[name] {
			return nil, fmt.Errorf("invalid or duplicate zpool entry")
		}
		seen[name] = true
		pools = append(pools, Pool{Name: name, Health: health})
	}
	if err := scan.Err(); err != nil {
		return nil, fmt.Errorf("scan zpool output: %w", err)
	}
	if len(pools) == 0 {
		return nil, errors.New("no ZFS pools reported")
	}
	return pools, nil
}

// ListPools invokes a fixed read-only command. No shell, network or mutation.
func ListPools(ctx context.Context) ([]Pool, error) {
	return DefaultCollector.ListPools(ctx)
}

// ListPools runs the configured read-only zpool list command.
func (c *PoolCollector) ListPools(ctx context.Context) ([]Pool, error) {
	out, err := c.Runner.Run(ctx, ZpoolPath, zpoolListHealthArgs)
	if err != nil {
		return nil, mapListPoolsError(err)
	}
	return ParseList(out.Stdout)
}

func mapListPoolsError(err error) error {
	var runErr *cmdrun.Error
	if !errors.As(err, &runErr) {
		return fmt.Errorf("zpool list failed: %w", err)
	}
	switch runErr.Kind {
	case cmdrun.NotAvailable:
		return fmt.Errorf("%s not available", runErr.Path)
	case cmdrun.Timeout:
		return fmt.Errorf("zpool timed out: %w", runErr.Cause)
	case cmdrun.Truncated:
		return fmt.Errorf("zpool list output truncated")
	case cmdrun.ReadError:
		return fmt.Errorf("zpool list failed: %w", runErr)
	case cmdrun.ExitError:
		return fmt.Errorf("zpool list failed: %w", runErr)
	default:
		return fmt.Errorf("zpool list failed: %w", err)
	}
}
