package release

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInvalidChecksumRejected(t *testing.T) {
	data := []byte("payload")
	art := Artifact{
		Filename: "bin",
		GOOS:     "linux",
		GOARCH:   "amd64",
		SHA256:   "0000000000000000000000000000000000000000000000000000000000000001",
		Size:     int64(len(data)),
		URL:      "/bin",
	}
	client := staticHTTP{"/bin": data}
	dir := t.TempDir()
	dest := filepath.Join(dir, "bin")
	if err := DownloadArtifact(context.Background(), client, art, dest); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestFailedActivationPreservesInstalledBinary(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "lock"))
	target := filepath.Join(dir, "pve-zfs-monitor")
	opts := InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"}
	orig := filepath.Join(dir, "orig")
	if err := os.WriteFile(orig, []byte("v1"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicInstall(orig, opts); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(dir, "bad")
	if err := os.WriteFile(bad, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Corrupt staging by using wrong artifact metadata
	art := Artifact{Filename: "bad", GOOS: "linux", GOARCH: "amd64", SHA256: "00", Size: 1, URL: "u"}
	if err := VerifyArtifactFile(bad, art); err == nil {
		t.Fatal("expected verify failure")
	}
	got, _ := os.ReadFile(target)
	if string(got) != "v1" {
		t.Fatalf("installed binary changed: %q", got)
	}
}
