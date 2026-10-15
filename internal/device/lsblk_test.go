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

func TestParseLsblkEmptyRejected(t *testing.T) {
	_, err := ParseLsblkJSON([]byte(`{"blockdevices":[]}`))
	if err == nil {
		t.Fatal("expected empty inventory error")
	}
}

func TestNVMeControllerPath(t *testing.T) {
	ctrl, id := controllerPathFromNamespace("/dev/nvme0n1p3")
	if ctrl != "/dev/nvme0" || id != "nvme0" {
		t.Fatalf("got %q %q", ctrl, id)
	}
}

func TestByIDDuplicatePhysicalKey(t *testing.T) {
	inv := buildTestInventory(t)
	if len(inv.PhysicalIndex) == 0 {
		t.Fatal("expected physical index")
	}
}
