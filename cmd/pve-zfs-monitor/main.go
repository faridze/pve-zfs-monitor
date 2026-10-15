// pve-zfs-monitor is a read-only health-check CLI for ZFS pools.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

var version = "dev"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		fmt.Fprintln(stdout, "Usage: pve-zfs-monitor <status|check|version> [--json]")
		return 0
	}
	if args[0] == "version" && len(args) == 1 {
		fmt.Fprintln(stdout, version)
		return 0
	}
	if args[0] != "status" && args[0] != "check" {
		fmt.Fprintln(stderr, "unknown command (see --help)")
		return 64
	}
	if len(args) > 2 || (len(args) == 2 && args[1] != "--json") {
		fmt.Fprintln(stderr, "invalid arguments (see --help)")
		return 64
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pools, err := zfs.ListPools(ctx)
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
