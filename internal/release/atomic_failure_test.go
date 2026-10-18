package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicInstallPreservesLiveBinaryOnStagingFailure(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(t.TempDir(), "lock"))
	dir := t.TempDir()
	target := filepath.Join(dir, "pve-zfs-monitor")
	opts := InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"}
	orig := filepath.Join(dir, "orig")
	if err := os.WriteFile(orig, []byte("live"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicInstall(orig, opts); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != "live" {
		t.Fatalf("target corrupted: %q", got)
	}
}

func TestRollbackUsesLockAndPreservesWhenMissingBackup(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(t.TempDir(), "lock"))
	dir := t.TempDir()
	target := filepath.Join(dir, "pve-zfs-monitor")
	opts := InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"}
	if err := os.WriteFile(target, []byte("v2"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(opts); err == nil {
		t.Fatal("expected rollback failure without backup")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v2" {
		t.Fatal("live binary changed after failed rollback")
	}
}
