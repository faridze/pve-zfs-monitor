// Package zfs contains read-only ZFS health collection and assessment.
package zfs

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

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
	const command = "/usr/sbin/zpool"
	if info, err := os.Stat(command); err != nil || info.IsDir() {
		return nil, fmt.Errorf("%s not available", command)
	}
	cmd := exec.CommandContext(ctx, command, "list", "-H", "-o", "name,health")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("zpool timed out: %w", ctx.Err())
		}
		return nil, fmt.Errorf("zpool list failed: %w", err)
	}
	return ParseList(output)
}
