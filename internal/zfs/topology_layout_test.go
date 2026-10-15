package zfs

import "testing"

func countNodes(node *VdevNode) int {
	if node == nil {
		return 0
	}
	n := 1
	for i := range node.Children {
		n += countNodes(&node.Children[i])
	}
	return n
}

func findChild(root *VdevNode, name string) *VdevNode {
	if root == nil {
		return nil
	}
	if root.Name == name {
		return root
	}
	for i := range root.Children {
		if got := findChild(&root.Children[i], name); got != nil {
			return got
		}
	}
	return nil
}

func TestLayoutSingleDiskPool(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_single_disk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := report.Pools[0].Root
	if countNodes(root) != 2 || len(root.Children) != 1 || root.Children[0].VdevClass != "leaf" {
		t.Fatalf("layout: %#v", root)
	}
}

func TestLayoutTwoMirrorVdevs(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_two_mirrors.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := report.Pools[0].Root
	if len(root.Children) != 2 {
		t.Fatalf("expected 2 top vdevs, got %d", len(root.Children))
	}
	for _, child := range root.Children {
		if child.VdevClass != "mirror" || len(child.Children) != 2 {
			t.Fatalf("mirror layout: %#v", child)
		}
	}
}

func TestLayoutRaidZPools(t *testing.T) {
	rz1, err := ParseStatus(loadFixture("pool_raidz1.txt"))
	if err != nil || rz1.Pools[0].Root.Children[0].VdevClass != "raidz" || len(rz1.Pools[0].Root.Children[0].Children) != 3 {
		t.Fatalf("raidz1: %#v err=%v", rz1.Pools[0].Root, err)
	}
	rz2, err := ParseStatus(loadFixture("pool_raidz2.txt"))
	if err != nil || len(rz2.Pools[0].Root.Children[0].Children) != 4 {
		t.Fatalf("raidz2 children: %d", len(rz2.Pools[0].Root.Children[0].Children))
	}
}

func TestLayoutMirroredLogsNested(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_mirrored_logs.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := report.Pools[0].Root
	logs := findChild(root, "logs")
	if logs == nil || logs.StateObserved {
		t.Fatalf("logs section: %#v", logs)
	}
	if logs.State != "" {
		t.Fatalf("section must not invent state, got %q", logs.State)
	}
	mirror := findChild(logs, "mirror-1")
	if mirror == nil || mirror.VdevClass != "mirror" || len(mirror.Children) != 2 {
		t.Fatalf("nested log mirror: %#v", mirror)
	}
}

func TestLayoutCacheAndSpecialSections(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_cache_special.txt"))
	if err != nil {
		t.Fatal(err)
	}
	root := report.Pools[0].Root
	cache := findChild(root, "cache")
	special := findChild(root, "special")
	if cache == nil || special == nil {
		t.Fatalf("missing sections: %#v", root.Children)
	}
	if findChild(special, "mirror-1") == nil || findChild(cache, "nvme0n1") == nil {
		t.Fatalf("nested auxiliary devices missing")
	}
}

func TestLayoutAuxiliaryDegradedSpare(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_aux_degraded.txt"))
	if err != nil {
		t.Fatal(err)
	}
	spares := findChild(report.Pools[0].Root, "spares")
	spare := spares.Children[0]
	if spare.State != "UNAVAIL" || spare.ReadErrors == nil {
		t.Fatalf("spare member: %#v", spare)
	}
}

func TestLayoutUnknownDraidType(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_draid_unknown.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if report.Pools[0].Root.Children[0].VdevClass != "unknown" {
		t.Fatalf("class: %q", report.Pools[0].Root.Children[0].VdevClass)
	}
}

func TestParseIncompletePrimaryMemberCountersRejected(t *testing.T) {
	_, err := ParseStatus(loadFixture("incomplete_member_counters.txt"))
	if err == nil {
		t.Fatal("expected validation failure for missing counters on data member")
	}
}

func TestListComparisonStatuses(t *testing.T) {
	report, err := ParseStatus(loadFixture("pool_single_disk.txt"))
	if err != nil {
		t.Fatal(err)
	}
	report.EnrichListHealth([]Pool{{Name: "solo", Health: "ONLINE"}})
	if report.ListComparisonStatus != ReportListComparisonComplete {
		t.Fatalf("got %q", report.ListComparisonStatus)
	}
	report.EnrichListHealth([]Pool{{Name: "solo", Health: "DEGRADED"}})
	if report.ListComparisonStatus != ReportListComparisonInconsistent {
		t.Fatalf("got %q", report.ListComparisonStatus)
	}
	report.MarkListHealthUnverified("test")
	if report.ListComparisonStatus != ReportListComparisonUnverified {
		t.Fatalf("got %q", report.ListComparisonStatus)
	}
}
