package device

import (
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestCorrelateDoesNotGuessPhantomPartition(t *testing.T) {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_single_disk.json"))
	if err != nil {
		t.Fatal(err)
	}
	topo := &zfs.TopologyReport{
		Pools: []zfs.PoolTopology{{
			Name: "tank",
			Root: &zfs.VdevNode{
				Name: "tank", VdevClass: "pool",
				Children: []zfs.VdevNode{{
					Name:          "/dev/sda999",
					DevicePath:    "/dev/sda999",
					VdevClass:     "disk",
					State:         "ONLINE",
					StateObserved: true,
				}},
			},
		}},
	}
	for _, m := range CorrelateTopology(topo, inv) {
		if m.Outcome == MappingMapped {
			t.Fatalf("phantom partition must not map to parent disk: %#v", m)
		}
		if m.Outcome != MappingMissing {
			t.Fatalf("expected missing, got %#v", m)
		}
	}
}

func TestCorrelatePartitionPresentMapsToVerifiedParent(t *testing.T) {
	topo, err := zfs.ParseStatus(loadZFSFixture("mirror_dev_partitions.txt"))
	if err != nil {
		t.Fatal(err)
	}
	inv := buildTestInventory(t)
	for _, m := range CorrelateTopology(topo, inv) {
		if m.Outcome != MappingMapped {
			continue
		}
		if m.DevicePath != "/dev/nvme0n1" && m.DevicePath != "/dev/nvme1n1" {
			t.Fatalf("unexpected mapped path %q", m.DevicePath)
		}
	}
}
