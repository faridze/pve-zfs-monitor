package device

import (
	"testing"

	"github.com/faridze/pve-zfs-monitor/internal/zfs"
)

func TestCorrelatePreservesUnknownTerminalVdev(t *testing.T) {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_single_disk.json"))
	if err != nil {
		t.Fatal(err)
	}
	topo := &zfs.TopologyReport{
		Pools: []zfs.PoolTopology{{
			Name: "lab",
			Root: &zfs.VdevNode{
				Name: "lab", VdevClass: "pool", State: "ONLINE", StateObserved: true,
				Children: []zfs.VdevNode{
					{
						Name: "mirror-0", VdevClass: "mirror", State: "ONLINE", StateObserved: true,
						Children: []zfs.VdevNode{
							{
								Name: "/dev/sda", DevicePath: "/dev/sda", VdevClass: "disk",
								State: "ONLINE", StateObserved: true,
							},
						},
					},
					{
						Name: "draid1-0", VdevClass: "unknown", State: "ONLINE", StateObserved: true,
					},
				},
			},
		}},
	}
	var unknownSeen, mappedSeen bool
	for _, m := range CorrelateTopology(topo, inv) {
		switch m.ZFSMemberName {
		case "draid1-0":
			unknownSeen = true
			if m.Outcome != MappingUnsupported {
				t.Fatalf("unknown terminal: %#v", m)
			}
			if m.DevicePath != "" {
				t.Fatal("must not invent device path for unknown vdev")
			}
		case "/dev/sda":
			mappedSeen = m.Outcome == MappingMapped
		}
	}
	if !unknownSeen {
		t.Fatal("unknown terminal vdev missing from member mappings")
	}
	if !mappedSeen {
		t.Fatal("expected mapped mirror member")
	}
	if mappingCompleteness(CorrelateTopology(topo, inv)) != ReportMappingIncomplete {
		t.Fatal("unknown terminal must prevent mapping_complete")
	}
}
