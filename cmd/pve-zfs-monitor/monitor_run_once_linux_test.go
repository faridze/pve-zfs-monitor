//go:build linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/labrun"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestMonitorRunOnceCLIRejectsUnsafePath(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWith(defaultRuntime, []string{
		"monitor", "run-once", "--lab", "--state-dir", "relative/path", "--json",
	}, &out, &errOut)
	if code != 64 {
		t.Fatalf("exit=%d err=%q", code, errOut.String())
	}
}

func TestMonitorRunOnceCLINoneDelivery(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := labrun.WriteLabMarker(dir); err != nil {
		t.Fatal(err)
	}
	cfg := runtimeConfig{
		listPools: func(ctx context.Context) ([]zfs.Pool, error) {
			return []zfs.Pool{{Name: "p1", Health: "DEGRADED"}}, nil
		},
		collectTopology: func(ctx context.Context) (*zfs.TopologyReport, error) { return nil, nil },
		collectDevices:  func(ctx context.Context) (*device.DeviceReport, error) { return nil, nil },
		timeout:         time.Second,
	}
	var out, errOut bytes.Buffer
	code := runWith(cfg, []string{
		"monitor", "run-once", "--lab", "--state-dir", dir, "--delivery", "none", "--json",
	}, &out, &errOut)
	if code != 0 && code != 2 && code != 3 {
		t.Fatalf("exit=%d err=%q out=%s", code, errOut.String(), out.String())
	}
	var report labrun.RunOnceReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json: %v", err)
	}
	if !report.Lab || report.DeliveryMode != "none" {
		t.Fatalf("report: %+v", report)
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		t.Fatalf("state not persisted: %v", err)
	}
}
