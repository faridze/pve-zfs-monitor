package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergePagesRejectsConflictingVersion(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))
	site := t.TempDir()
	ver := "1.0.0-test"
	existing := filepath.Join(site, "lab", ver)
	_ = os.MkdirAll(existing, 0o755)
	binA := []byte{1, 2, 3}
	_ = writeVersionBundle(t, existing, pub, ver, binA)
	_ = os.WriteFile(filepath.Join(existing, "pve-zfs-monitor-linux-arm64"), binA, 0o644)

	newDir := filepath.Join(t.TempDir(), ver)
	_ = os.MkdirAll(newDir, 0o755)
	binB := []byte{9, 9, 9}
	_ = writeVersionBundle(t, newDir, pub, ver, binB)
	_ = os.WriteFile(filepath.Join(newDir, "pve-zfs-monitor-linux-arm64"), binB, 0o644)

	pubPath := filepath.Join(t.TempDir(), "pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)

	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	out, err := exec.Command("bash", merge, site, newDir, pubPath, root).CombinedOutput()
	if err == nil {
		t.Fatalf("expected refuse overwrite, out=%s", out)
	}
	if !strings.Contains(string(out), "refusing to overwrite") {
		t.Fatalf("unexpected output: %s", out)
	}
}

func TestRecordPublishedLabVersionIdempotent(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	site := t.TempDir()
	script := filepath.Join(root, "release", "signing", "record-published-lab-version.sh")
	for i := 0; i < 2; i++ {
		if out, err := exec.Command("bash", script, site, "1.0.0-rc1").CombinedOutput(); err != nil {
			t.Fatalf("run %d: %v\n%s", i, err, out)
		}
	}
	list, err := os.ReadFile(filepath.Join(site, "lab", ".published-versions"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(list), "1.0.0-rc1") != 1 {
		t.Fatalf("expected single entry, got %q", list)
	}
}
