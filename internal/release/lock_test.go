package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockRejectsSymlinkTarget(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	real := filepath.Join(dir, "real.lock")
	if err := os.WriteFile(real, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.lock")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", link)
	_, err := AcquireUpdateLock("")
	if err == nil {
		t.Fatal("expected symlink lock rejection")
	}
}

func TestLockDoesNotTruncatePayload(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	path := filepath.Join(dir, "install.lock")
	payload := "preserve-me"
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	l1, err := AcquireUpdateLock(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatalf("lock open truncated file: got %q", got)
	}
	_ = l1.Release()
}

func TestProductionRefusesCustomLockPath(t *testing.T) {
	os.Unsetenv("PVE_ZFS_MONITOR_HERMETIC_TEST")
	os.Unsetenv("PVE_ZFS_MONITOR_LOCK_PATH")
	err := validateLockFilePath("/tmp/evil.lock")
	if err == nil {
		t.Fatal("expected production lock path rejection")
	}
}
