package device

import (
	"strings"
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func buildTestInventory(t *testing.T) *Inventory {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_nvme_mirror.json"))
	if err != nil {
		t.Fatal(err)
	}
	byID, err := ParseByIDLines(strings.Split(string(loadFixture("byid_nvme_mirror.txt")), "\n"))
	if err != nil {
		t.Fatal(err)
	}
	ApplyByIDLinks(inv, byID)
	return inv
}

func TestCorrelateFullByIDPath(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("mirror_by_id.txt"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseLsblkJSON(loadFixture("lsblk_nvme_mirror.json"))
	if err != nil {
		t.Fatal(err)
	}
	byID, err := ParseByIDLines(strings.Split(string(loadFixture("byid_mirror_by_id.txt")), "\n"))
	if err != nil {
		t.Fatal(err)
	}
	ApplyByIDLinks(inv, byID)
	mapped := 0
	for _, m := range CorrelateTopology(topo, inv) {
		if m.Outcome == MappingMapped {
			mapped++
		}
	}
	if mapped < 2 {
		t.Fatalf("by-id full path mapping failed: %#v", CorrelateTopology(topo, inv))
	}
}

func TestCorrelateNVMeMirrorPartitions(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("mirror_dev_partitions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	mappings := CorrelateTopology(topo, buildTestInventory(t))
	mapped := 0
	for _, m := range mappings {
		if m.Outcome == MappingMapped {
			mapped++
		}
		if m.Outcome == MappingAmbiguous {
			t.Fatalf("unexpected ambiguous: %#v", m)
		}
	}
	if mapped != 2 {
		t.Fatalf("expected 2 mapped members, got %#v", mappings)
	}
}

func TestCorrelateGUIDUnavailableMissing(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("member_guid_unavail.txt"))
	if err != nil {
		t.Fatal(err)
	}
	var missing bool
	for _, m := range CorrelateTopology(topo, buildTestInventory(t)) {
		if m.ZFSState == "UNAVAIL" {
			if m.Outcome != MappingMissing {
				t.Fatalf("unavail GUID must be missing, got %#v", m)
			}
			missing = true
		}
		if m.ZFSState == "ONLINE" && IsGUIDMember(m.ZFSMemberName) && m.Outcome == MappingMapped {
			t.Fatalf("must not map GUID member without path evidence: %#v", m)
		}
	}
	if !missing {
		t.Fatal("expected UNAVAIL member")
	}
}

func TestCorrelateAmbiguousDualPaths(t *testing.T) {
	inv := buildTestInventory(t)
	topo := &zfs.TopologyReport{
		Pools: []zfs.PoolTopology{{
			Name: "tank",
			Root: &zfs.VdevNode{
				Name: "tank", VdevClass: "pool",
				Children: []zfs.VdevNode{{
					Name:          "nvme-eui.abc111",
					DevicePath:    "/dev/nvme1n1p3",
					VdevClass:     "disk",
					State:         "ONLINE",
					StateObserved: true,
					ReadErrors:    uint64Ptr(0),
					WriteErrors:   uint64Ptr(0),
					CksumErrors:   uint64Ptr(0),
				}},
			},
		}},
	}
	for _, m := range CorrelateTopology(topo, inv) {
		if m.Outcome != MappingAmbiguous {
			t.Fatalf("expected ambiguous, got %#v", m)
		}
	}
}

func TestCorrelateRaidZMembers(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("pool_raidz1.txt"))
	if err != nil {
		t.Fatal(err)
	}
	inv, err := ParseLsblkJSON(loadFixture("lsblk_raidz.json"))
	if err != nil {
		t.Fatal(err)
	}
	mapped := 0
	for _, m := range CorrelateTopology(topo, inv) {
		if m.Outcome == MappingMapped {
			mapped++
		}
	}
	if mapped != 3 {
		t.Fatalf("raidz mapped=%d mappings=%#v", mapped, CorrelateTopology(topo, inv))
	}
}

func uint64Ptr(v uint64) *uint64 { return &v }
