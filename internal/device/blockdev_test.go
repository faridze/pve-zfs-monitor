package device

import "testing"

func stubVerifierFromInventory(inv *Inventory) StubDeviceVerifier {
	allowed := make(map[string]bool)
	for p := range inv.Devices {
		allowed[p] = true
	}
	return StubDeviceVerifier{Allowed: allowed}
}

func TestResolveSMARTTargetRejectsPhantomPartition(t *testing.T) {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_nvme_mirror.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = ResolveSMARTTarget("/dev/nvme0n1p999", inv, StubDeviceVerifier{Allowed: map[string]bool{"/dev/nvme0n1": true}})
	if err == nil {
		t.Fatal("expected reject for path not in inventory")
	}
}

func TestResolveSMARTTargetPartitionToVerifiedParent(t *testing.T) {
	inv, err := ParseLsblkJSON(loadFixture("lsblk_nvme_mirror.json"))
	if err != nil {
		t.Fatal(err)
	}
	target, err := ResolveSMARTTarget("/dev/nvme0n1p3", inv, stubVerifierFromInventory(inv))
	if err != nil {
		t.Fatal(err)
	}
	if target != "/dev/nvme0n1" {
		t.Fatalf("got %q", target)
	}
}
