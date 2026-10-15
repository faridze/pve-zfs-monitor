package device

import "testing"

func TestValidateDevicePathAllowlist(t *testing.T) {
	ok := []string{"/dev/nvme0n1", "/dev/nvme0n1p3", "/dev/sda", "/dev/disk/by-id/nvme-eui.abc"}
	for _, p := range ok {
		if err := ValidateDevicePath(p); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	bad := []string{"/dev/../etc/passwd", "/tmp/foo", "/dev/foo bar", "/dev/null", "/dev/zero", "/dev/random"}
	for _, p := range bad {
		if err := ValidateDevicePath(p); err == nil {
			t.Fatalf("expected reject %s", p)
		}
	}
}
