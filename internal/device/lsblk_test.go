package device

import "testing"

func TestParseLsblkNVMeMirror(t *testing.T) {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_nvme_mirror.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Devices) < 4 {
		t.Fatalf("devices: %d", len(inv.Devices))
	}
	if inv.Devices["/dev/nvme0n1"].Transport != TransportNVMe {
		t.Fatalf("transport: %#v", inv.Devices["/dev/nvme0n1"])
	}
}

func TestByIDDuplicatePhysicalKey(t *testing.T) {
	inv := buildTestInventory(t)
	if len(inv.PhysicalIndex) == 0 {
		t.Fatal("expected physical index")
	}
}
