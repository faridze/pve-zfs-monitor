package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/faridze/pve-zfs-monitor/internal/device"
	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func testRuntime(pools []zfs.Pool, err error) runtimeConfig {
	return runtimeConfig{
		listPools: func(context.Context) ([]zfs.Pool, error) {
			return pools, err
		},
		collectTopology: func(context.Context) (*zfs.TopologyReport, error) {
			return &zfs.TopologyReport{}, nil
		},
		collectDevices: func(context.Context) (*device.DeviceReport, error) {
			return &device.DeviceReport{}, nil
		},
		timeout: time.Second,
	}
}

func TestTopologyJSONCommand(t *testing.T) {
	cfg := testRuntime(nil, nil)
	cfg.collectTopology = func(context.Context) (*zfs.TopologyReport, error) {
		return &zfs.TopologyReport{
			Pools: []zfs.PoolTopology{{
				Name:  "rpool",
				State: "ONLINE",
				Root:  &zfs.VdevNode{Name: "rpool", State: "ONLINE", VdevClass: "pool"},
			}},
		}, nil
	}
	var out, errOut bytes.Buffer
	code := runWith(cfg, []string{"topology", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("topology exit=%d err=%q", code, errOut.String())
	}
	var report zfs.TopologyReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(report.Pools) != 1 || report.Pools[0].Name != "rpool" {
		t.Fatalf("report: %#v", report)
	}
}

func TestDevicesJSONCommand(t *testing.T) {
	cfg := runtimeConfig{
		collectDevices: func(context.Context) (*device.DeviceReport, error) {
			return &device.DeviceReport{MemberMappings: []device.MemberMapping{{Outcome: device.MappingMissing}}}, nil
		},
		timeout: time.Second,
	}
	var out, errOut bytes.Buffer
	code := runWith(cfg, []string{"devices", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("devices exit=%d err=%q", code, errOut.String())
	}
}

func TestTopologyRequiresJSON(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWith(testRuntime(nil, nil), []string{"topology"}, &out, &errOut)
	if code != 64 {
		t.Fatalf("exit=%d want 64", code)
	}
}

func TestSafeCLICommands(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"version"}, &out, &errOut); code != 0 || out.Len() == 0 {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
	out.Reset()
	if code := run([]string{"invalid"}, &out, &errOut); code != 64 {
		t.Fatalf("unknown command: code=%d", code)
	}
	errOut.Reset()
	if code := run([]string{"check", "--dangerous"}, &out, &errOut); code != 64 || !strings.Contains(errOut.String(), "invalid") {
		t.Fatalf("invalid arguments: code=%d stderr=%q", code, errOut.String())
	}
}

func TestCheckExitCodes(t *testing.T) {
	cases := []struct {
		name  string
		pools []zfs.Pool
		want  int
	}{
		{"healthy", []zfs.Pool{{Name: "rpool", Health: "ONLINE"}}, 0},
		{"degraded", []zfs.Pool{{Name: "rpool", Health: "DEGRADED"}}, 2},
		{"faulted", []zfs.Pool{{Name: "rpool", Health: "FAULTED"}}, 2},
		{"unknown health", []zfs.Pool{{Name: "rpool", Health: "FUTURE"}}, 3},
		{"mixed unhealthy", []zfs.Pool{{Name: "a", Health: "ONLINE"}, {Name: "b", Health: "DEGRADED"}}, 2},
		{"mixed unknown", []zfs.Pool{{Name: "a", Health: "ONLINE"}, {Name: "b", Health: "WEIRD"}}, 3},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := runWith(testRuntime(tc.pools, nil), []string{"check"}, &out, &errOut)
		if code != tc.want {
			t.Fatalf("%s: check exit=%d want=%d out=%q err=%q", tc.name, code, tc.want, out.String(), errOut.String())
		}
	}
}

func TestStatusDoesNotFailOnUnhealthy(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runWith(testRuntime([]zfs.Pool{{Name: "rpool", Health: "FAULTED"}}, nil), []string{"status"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("status exit=%d want 0", code)
	}
	if !strings.Contains(out.String(), "FAULTED") || !strings.Contains(out.String(), "Overall: unhealthy") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestStatusJSONSchema(t *testing.T) {
	pools := []zfs.Pool{{Name: "rpool", Health: "ONLINE"}, {Name: "data", Health: "DEGRADED"}}
	var out, errOut bytes.Buffer
	code := runWith(testRuntime(pools, nil), []string{"status", "--json"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("status --json exit=%d err=%q", code, errOut.String())
	}
	var payload struct {
		Pools []struct {
			Name   string `json:"name"`
			Health string `json:"health"`
		} `json:"pools"`
		Overall string `json:"overall"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid json: %v body=%q", err, out.String())
	}
	if len(payload.Pools) != 2 || payload.Pools[0].Name != "rpool" || payload.Pools[1].Health != "DEGRADED" {
		t.Fatalf("unexpected pools: %#v", payload.Pools)
	}
	if payload.Overall != "unhealthy" {
		t.Fatalf("overall=%q want unhealthy", payload.Overall)
	}
}

func TestCollectionErrorsExitUnknown(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"missing zpool", errors.New("/usr/sbin/zpool not available")},
		{"zpool failure", errors.New("zpool list failed: exit status 1")},
		{"timeout", errors.New("zpool timed out: context deadline exceeded")},
		{"truncated", errors.New("zpool list output truncated")},
		{"malformed", errors.New("malformed zpool output")},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := runWith(testRuntime(nil, tc.err), []string{"check"}, &out, &errOut)
		if code != 3 {
			t.Fatalf("%s: exit=%d want 3", tc.name, code)
		}
		if !strings.Contains(errOut.String(), "ZFS health UNKNOWN") {
			t.Fatalf("%s: missing UNKNOWN prefix: %q", tc.name, errOut.String())
		}
	}
}
