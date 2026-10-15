package zfs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/cmdrun"
)

func loadFixture(name string) []byte {
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join("internal", "zfs", "testdata", name)
		data, err = os.ReadFile(path)
	}
	if err != nil {
		panic(err)
	}
	return data
}

func TestParseMirrorHealthy(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Pools) != 1 || report.Pools[0].State != "ONLINE" {
		t.Fatalf("unexpected pool: %#v", report.Pools)
	}
	root := report.Pools[0].Root
	if root == nil || len(root.Children) != 1 || root.Children[0].VdevClass != "mirror" {
		t.Fatalf("unexpected root: %#v", root)
	}
	if report.Pools[0].Scan == nil || report.Pools[0].Scan.Phase != "completed" {
		t.Fatalf("scan: %#v", report.Pools[0].Scan)
	}
	if root.Children[0].Children[0].DevicePath != "/dev/nvme0n1" {
		t.Fatalf("missing device path: %#v", root.Children[0].Children[0])
	}
}

func TestParseDegradedMirror(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_degraded.txt"))
	if err != nil {
		t.Fatal(err)
	}
	pool := report.Pools[0]
	if pool.State != "DEGRADED" || pool.Scan.Phase != "in_progress" {
		t.Fatalf("pool: %#v", pool)
	}
	mirror := pool.Root.Children[0]
	if len(mirror.Children) != 2 {
		t.Fatalf("children: %#v", mirror.Children)
	}
	if mirror.Children[1].State != "UNAVAIL" {
		t.Fatalf("member state: %#v", mirror.Children[1])
	}
}

func TestParseFaultedCounters(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_faulted_counters.txt"))
	if err != nil {
		t.Fatal(err)
	}
	sda := report.Pools[0].Root.Children[0].Children[0]
	if sda.State != "FAULTED" || sda.ReadErrors == nil || *sda.ReadErrors != 4 {
		t.Fatalf("sda: %#v", sda)
	}
	if report.Pools[0].Scan.Errors == nil || *report.Pools[0].Scan.Errors != 3 {
		t.Fatalf("scan errors: %#v", report.Pools[0].Scan)
	}
}

func TestParseMultiplePools(t *testing.T) {
	report, err := ParseStatus(loadFixture("multi_pool.txt"))
	if err != nil || len(report.Pools) != 2 {
		t.Fatalf("pools: %#v err=%v", report, err)
	}
	if report.Pools[0].Name != "rpool" || report.Pools[1].Name != "backup" {
		t.Fatalf("names: %#v", report.Pools)
	}
}

func TestParseNestedRaidZ(t *testing.T) {
	report, err := ParseStatus(loadFixture("raidz_nested.txt"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := report.Pools[0].Root.Children[0]
	if mirror.VdevClass != "mirror" || len(mirror.Children) != 2 {
		t.Fatalf("mirror: %#v", mirror)
	}
	if mirror.Children[0].VdevClass != "raidz" || mirror.Children[1].VdevClass != "raidz" {
		t.Fatalf("raidz children: %#v", mirror.Children)
	}
	if report.Pools[0].Scan.Operation != "resilver" {
		t.Fatalf("scan: %#v", report.Pools[0].Scan)
	}
}

func TestParseScrubCancelled(t *testing.T) {
	report, err := ParseStatus(loadFixture("scrub_failed.txt"))
	if err != nil || report.Pools[0].Scan.Phase != "cancelled" {
		t.Fatalf("scan: %#v err=%v", report.Pools[0].Scan, err)
	}
}

func TestParseLegacyScrubKey(t *testing.T) {
	report, err := ParseStatus(loadFixture("legacy_scrub_key.txt"))
	if err != nil || report.Pools[0].Scan.Phase != "completed" {
		t.Fatalf("scan: %#v err=%v", report.Pools[0].Scan, err)
	}
}

func TestParseMalformedFailsClosed(t *testing.T) {
	_, err := ParseStatus(loadFixture("malformed.txt"))
	if err == nil {
		t.Fatal("expected parse failure")
	}
}

func TestParseTruncatedFailsClosed(t *testing.T) {
	data := loadFixture("mirror_healthy.txt")
	truncated := data[:len(data)/2]
	_, err := ParseStatus(truncated)
	if err == nil {
		t.Fatal("expected failure on truncated fixture")
	}
}

func TestParseEmptyFails(t *testing.T) {
	if _, err := ParseStatus(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestTopologyCollectorStub(t *testing.T) {
	stub := &stubRunner{out: cmdrun.Output{Stdout: loadFixture("mirror_healthy.txt")}}
	c := NewTopologyCollector(stub)
	report, err := c.Collect(context.Background())
	if err != nil || len(report.Pools) != 1 {
		t.Fatalf("collect: %#v err=%v", report, err)
	}
	if stub.lastPath != ZpoolPath || len(stub.lastArgs) != 2 || stub.lastArgs[0] != "status" || stub.lastArgs[1] != "-v" {
		t.Fatalf("unexpected invocation: path=%q args=%v", stub.lastPath, stub.lastArgs)
	}
}

func TestTopologyCollectorPermissionFailure(t *testing.T) {
	c := NewTopologyCollector(&stubRunner{err: &cmdrun.Error{
		Kind:     cmdrun.ExitError,
		Path:     ZpoolPath,
		ExitCode: 1,
		Cause:    errors.New("permission denied"),
	}})
	_, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "zpool status failed") {
		t.Fatalf("expected status failure, got %v", err)
	}
}

func TestTopologyCollectorTimeout(t *testing.T) {
	c := NewTopologyCollector(&stubRunner{err: &cmdrun.Error{
		Kind:  cmdrun.Timeout,
		Path:  ZpoolPath,
		Cause: context.DeadlineExceeded,
	}})
	_, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestEnrichListHealth(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.EnrichListHealth([]Pool{{Name: "rpool", Health: "ONLINE"}})
	if report.Pools[0].ListHealth != "ONLINE" || report.Pools[0].HealthMatch == nil || !*report.Pools[0].HealthMatch {
		t.Fatalf("health enrich: %#v", report.Pools[0])
	}
}
