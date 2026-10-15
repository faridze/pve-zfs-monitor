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
	mirror := report.Pools[0].Root.Children[0]
	if len(mirror.Children) != 2 {
		t.Fatalf("mirror children: %#v", mirror.Children)
	}
	if mirror.Children[0].Children[0].Name != "/dev/nvme0n1" {
		t.Fatalf("expected nested /dev member, got %#v", mirror.Children[0].Children[0])
	}
}

func TestParseDirectDevMirrorMembers(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_dev_partitions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := report.Pools[0].Root.Children[0]
	if len(mirror.Children) != 2 {
		t.Fatalf("expected 2 members, got %#v", mirror.Children)
	}
	for _, child := range mirror.Children {
		if child.VdevClass != "disk" || !strings.HasPrefix(child.Name, "/dev/nvme") {
			t.Fatalf("unexpected member: %#v", child)
		}
		if child.ReadErrors == nil || child.WriteErrors == nil || child.CksumErrors == nil {
			t.Fatalf("expected known zero counters, got %#v", child)
		}
	}
}

func TestParseByIDMirrorMembers(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_by_id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := report.Pools[0].Root.Children[0]
	if len(mirror.Children) != 2 || !strings.HasPrefix(mirror.Children[0].Name, "nvme-") {
		t.Fatalf("unexpected members: %#v", mirror.Children)
	}
}

func TestParseGUIDMemberUnavailable(t *testing.T) {
	report, err := ParseStatus(loadFixture("member_guid_unavail.txt"))
	if err != nil {
		t.Fatal(err)
	}
	member := report.Pools[0].Root.Children[0].Children[0]
	if member.State != "UNAVAIL" || !strings.Contains(member.StateDetail, "was /dev/nvme0n1") {
		t.Fatalf("member: %#v", member)
	}
}

func TestParseAuxiliarySpares(t *testing.T) {
	report, err := ParseStatus(loadFixture("auxiliary_spares.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := report.Pools[0].Root
	var spares *VdevNode
	for i := range root.Children {
		if root.Children[i].Name == "spares" {
			spares = &root.Children[i]
			break
		}
	}
	if spares == nil || spares.VdevClass != "spare" {
		t.Fatalf("missing spares section: %#v", root.Children)
	}
	if len(spares.Children) != 2 {
		t.Fatalf("spares children: %#v", spares.Children)
	}
	if spares.Children[0].ReadErrors != nil || spares.Children[0].CksumErrors != nil {
		t.Fatalf("AVAIL spare counters must remain unknown: %#v", spares.Children[0])
	}
}

func TestParseDegradedMirror(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_degraded.txt"))
	if err != nil {
		t.Fatal(err)
	}
	mirror := report.Pools[0].Root.Children[0]
	if len(mirror.Children) != 2 || mirror.Children[1].State != "UNAVAIL" {
		t.Fatalf("children: %#v", mirror.Children)
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
}

func TestParseMultiplePools(t *testing.T) {
	report, err := ParseStatus(loadFixture("multi_pool.txt"))
	if err != nil || len(report.Pools) != 2 {
		t.Fatalf("pools: %#v err=%v", report, err)
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

func TestParseDuplicatePoolRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("duplicate_pool_blocks.txt"))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate pool error, got %v", err)
	}
}

func TestParseMirrorSingleMemberRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("mirror_single_member.txt"))
	if err == nil || !strings.Contains(err.Error(), "fewer than two members") {
		t.Fatalf("expected mirror validation error, got %v", err)
	}
}

func TestParseRootNameMismatchRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("root_name_mismatch.txt"))
	if err == nil || !strings.Contains(err.Error(), "does not match pool name") {
		t.Fatalf("expected root mismatch error, got %v", err)
	}
}

func TestParseTruncatedFailsClosed(t *testing.T) {
	data := loadFixture("mirror_healthy.txt")
	_, err := ParseStatus(data[:len(data)/2])
	if err == nil {
		t.Fatal("expected failure on truncated fixture")
	}
}

func TestParseEmptyFails(t *testing.T) {
	if _, err := ParseStatus(nil); err == nil {
		t.Fatal("expected error")
	}
}

func TestTopologyCollectorStubWithoutList(t *testing.T) {
	stub := &stubRunner{out: cmdrun.Output{Stdout: loadFixture("mirror_healthy.txt")}}
	c := NewTopologyCollectorWithList(stub, nil)
	report, err := c.Collect(context.Background())
	if err != nil || len(report.Pools) != 1 {
		t.Fatalf("collect: %#v err=%v", report, err)
	}
	if report.Pools[0].ListHealthStatus != ListHealthUnverified {
		t.Fatalf("expected unverified list health, got %q", report.Pools[0].ListHealthStatus)
	}
}

func TestTopologyCollectorPermissionFailure(t *testing.T) {
	c := NewTopologyCollectorWithList(&stubRunner{err: &cmdrun.Error{
		Kind:     cmdrun.ExitError,
		Path:     ZpoolPath,
		ExitCode: 1,
		Cause:    errors.New("permission denied"),
	}}, nil)
	_, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "zpool status failed") {
		t.Fatalf("expected status failure, got %v", err)
	}
}

func TestTopologyCollectorTimeout(t *testing.T) {
	c := NewTopologyCollectorWithList(&stubRunner{err: &cmdrun.Error{
		Kind:  cmdrun.Timeout,
		Path:  ZpoolPath,
		Cause: context.DeadlineExceeded,
	}}, nil)
	_, err := c.Collect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestEnrichListHealthConsistent(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.EnrichListHealth([]Pool{{Name: "rpool", Health: "ONLINE"}})
	if report.Pools[0].ListHealthStatus != ListHealthVerifiedConsistent {
		t.Fatalf("status: %#v", report.Pools[0])
	}
}

func TestEnrichListHealthBidirectionalMismatch(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.EnrichListHealth([]Pool{{Name: "rpool", Health: "ONLINE"}, {Name: "ghost", Health: "ONLINE"}})
	if len(report.ReportIssues) == 0 {
		t.Fatal("expected report issue for list-only pool")
	}
}

func TestEnrichListHealthInconsistentState(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.EnrichListHealth([]Pool{{Name: "rpool", Health: "DEGRADED"}})
	if report.Pools[0].ListHealthStatus != ListHealthVerifiedInconsistent {
		t.Fatalf("status: %#v", report.Pools[0])
	}
}

func TestMarkListHealthUnverified(t *testing.T) {
	report, err := ParseStatus(loadFixture("mirror_healthy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.MarkListHealthUnverified("list unavailable")
	if report.Pools[0].ListHealthStatus != ListHealthUnverified {
		t.Fatalf("got %q", report.Pools[0].ListHealthStatus)
	}
}
