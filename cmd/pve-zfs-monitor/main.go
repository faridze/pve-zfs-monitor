// pve-zfs-monitor is a read-only health-check CLI for ZFS pools.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

var version = "dev"

type runtimeConfig struct {
	listPools       func(context.Context) ([]zfs.Pool, error)
	collectTopology func(context.Context) (*zfs.TopologyReport, error)
	collectDevices  func(context.Context) (*device.DeviceReport, error)
	timeout         time.Duration
	devicesTimeout  time.Duration
}

var defaultRuntime = runtimeConfig{
	listPools:       zfs.ListPools,
	collectTopology: zfs.CollectTopology,
	collectDevices: func(ctx context.Context) (*device.DeviceReport, error) {
		return device.DefaultCollector().Collect(ctx)
	},
	timeout:        10 * time.Second,
	devicesTimeout: 120 * time.Second,
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWith(defaultRuntime, args, stdout, stderr)
}

func runWith(cfg runtimeConfig, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(stdout, "Usage: pve-zfs-monitor <status|check|topology|devices|version> [--json]")
		return 0
	}
	if args[0] == "version" && len(args) == 1 {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if args[0] == "topology" {
		return runTopology(cfg, args, stdout, stderr)
	}
	if args[0] == "devices" {
		return runDevices(cfg, args, stdout, stderr)
	}
	if args[0] != "status" && args[0] != "check" {
		fmt.Fprintln(stderr, "unknown command (see --help)")
		return 64
	}
	if len(args) > 2 || (len(args) == 2 && args[1] != "--json") {
		fmt.Fprintln(stderr, "invalid arguments (see --help)")
		return 64
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	pools, err := cfg.listPools(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "ZFS health UNKNOWN: %v\n", err)
		return 3
	}

	result := zfs.Assess(pools)
	if len(args) == 2 {
		if err := json.NewEncoder(stdout).Encode(struct {
			Pools   []zfs.Pool `json:"pools"`
			Overall string     `json:"overall"`
		}{pools, result.Label()}); err != nil {
			fmt.Fprintf(stderr, "failed to encode status: %v\n", err)
			return 3
		}
	} else {
		for _, pool := range pools {
			fmt.Fprintf(stdout, "%s: %s\n", pool.Name, pool.Health)
		}
		fmt.Fprintf(stdout, "Overall: %s\n", result.Label())
	}
	if args[0] == "check" {
		return int(result)
	}
	return 0 // status is informational; check has monitoring exit codes.
}

func runTopology(cfg runtimeConfig, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[1] != "--json" {
		fmt.Fprintln(stderr, "topology requires --json (see --help)")
		return 64
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout)
	defer cancel()
	report, err := cfg.collectTopology(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "ZFS topology UNKNOWN: %v\n", err)
		return 3
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintf(stderr, "failed to encode topology: %v\n", err)
		return 3
	}
	return 0
}

func runDevices(cfg runtimeConfig, args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 || args[1] != "--json" {
		fmt.Fprintln(stderr, "devices requires --json (see --help)")
		return 64
	}
	budget := cfg.devicesTimeout
	if budget == 0 {
		budget = cfg.timeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	report, err := cfg.collectDevices(ctx)
	if err != nil {
		fmt.Fprintf(stderr, "device health UNKNOWN: %v\n", err)
		return 3
	}
	if err := json.NewEncoder(stdout).Encode(report); err != nil {
		fmt.Fprintf(stderr, "failed to encode devices: %v\n", err)
		return 3
	}
	return 0
}
