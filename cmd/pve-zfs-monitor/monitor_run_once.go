package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/faridze/pve-zfs-monitor/internal/labrun"
	"github.com/faridze/pve-zfs-monitor/internal/monitor"
)

func runMonitorRunOnce(cfg runtimeConfig, args []string, stdout, stderr io.Writer) int {
	opts, err := parseMonitorRunOnceArgs(args)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 64
	}
	if !opts.lab {
		fmt.Fprintln(stderr, "monitor run-once requires --lab")
		return 64
	}
	mode, err := labrun.ParseDeliveryMode(opts.delivery)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 64
	}
	if err := labrun.ValidateLabStateDir(opts.stateDir); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 64
	}
	collectors := labrun.Collectors{
		ListPools:       cfg.listPools,
		CollectTopology: cfg.collectTopology,
		CollectDevices:  cfg.collectDevices,
		PoolTimeout:     cfg.timeout,
		DevicesTimeout:  cfg.devicesTimeout,
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.devicesTimeout+cfg.timeout)
	defer cancel()

	report, err := labrun.RunOnce(ctx, true, opts.stateDir, mode, collectors, "lab")
	if opts.json {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(stderr, "encode: %v\n", err)
			return 3
		}
	}
	if err != nil {
		fmt.Fprintf(stderr, "monitor run-once failed: %v\n", err)
		if !report.StatePersisted {
			return 3
		}
		return 3
	}
	if !opts.json {
		fmt.Fprintf(stdout, "Overall: %s\nPending: %d\nEvents: %d\n",
			report.Overall, report.PendingNotifications, report.EventsEmittedCount)
		if mode == labrun.DeliveryFake {
			fmt.Fprintf(stdout, "Simulated deliveries: %d\n", report.SimulatedDeliveries)
		}
	}
	switch report.Overall {
	case monitor.Critical:
		return 2
	case monitor.Unknown:
		return 3
	default:
		return 0
	}
}

type monitorRunOnceOpts struct {
	lab      bool
	stateDir string
	delivery string
	json     bool
}

func parseMonitorRunOnceArgs(args []string) (monitorRunOnceOpts, error) {
	var o monitorRunOnceOpts
	o.delivery = "none"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--lab":
			o.lab = true
		case "--state-dir":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--state-dir requires a value")
			}
			i++
			o.stateDir = args[i]
		case "--delivery":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--delivery requires a value")
			}
			i++
			o.delivery = args[i]
		case "--json":
			o.json = true
		default:
			return o, fmt.Errorf("unknown flag %q", args[i])
		}
	}
	if o.stateDir == "" {
		return o, fmt.Errorf("--state-dir is required")
	}
	return o, nil
}
