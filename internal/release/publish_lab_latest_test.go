package release

import (
	"context"
	"crypto/ed25519"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishLabLatestPreservesPreviousOnFailure(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 required")
	}

	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))
	labRoot := t.TempDir()
	pubPath := filepath.Join(labRoot, "pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)

	// Existing latest (rc1)
	latestDir := filepath.Join(labRoot, "latest")
	_ = os.MkdirAll(latestDir, 0o755)
	rc1Dir := filepath.Join(labRoot, "1.0.0-rc1")
	_ = os.MkdirAll(rc1Dir, 0o755)
	rc1Bin := []byte{0x7f, 'E', 'L', 'F', '1'}
	_ = writeVersionBundle(t, rc1Dir, pub, "1.0.0-rc1", rc1Bin)
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json"), filepath.Join(latestDir, "manifest.json"))
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json.sig.json"), filepath.Join(latestDir, "manifest.json.sig.json"))

	rc2Dir := filepath.Join(labRoot, "1.0.0-rc2")
	_ = os.MkdirAll(rc2Dir, 0o755)
	_ = writeVersionBundle(t, rc2Dir, pub, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F', '2'})

	publish := filepath.Join(root, "release", "signing", "publish-lab-latest.sh")
	cmd := exec.Command("bash", publish, rc2Dir, labRoot, pubPath)
	cmd.Env = append(os.Environ(), "PVE_ZFS_MONITOR_PUBLISH_TEST_FAIL=before-promote")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected publish failure, out=%s", out)
	}

	got, err := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "1.0.0-rc1") {
		t.Fatalf("latest must remain rc1 after failed promote, got %s", got)
	}
	if _, err := os.Stat(filepath.Join(labRoot, "latest")); err != nil {
		t.Fatal("lab/latest directory must remain present")
	}

	cmd2 := exec.Command("bash", publish, rc2Dir, labRoot, pubPath)
	out2, err := cmd2.CombinedOutput()
	if err != nil {
		t.Fatalf("successful promote failed: %v\n%s", err, out2)
	}
	got2, _ := os.ReadFile(filepath.Join(latestDir, "manifest.json"))
	if !strings.Contains(string(got2), "1.0.0-rc2") {
		t.Fatalf("expected rc2 after success, got %s", got2)
	}
}

func TestLatestManifestSignatureMismatchFailsClosed(t *testing.T) {
	pub, _ := testSigningKeypair()
	binRC1 := []byte{0x7f, 'E', 'L', 'F', '1'}
	_, rc1Raw, rc1Sig := buildLabManifestAtVersion(t, "https://releases.example.test", "1.0.0-rc1", binRC1)
	_, rc2Raw, _ := buildLabManifestAtVersion(t, "https://releases.example.test", "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F', '2'})

	client := staticHTTP{
		"/lab/latest/manifest.json":                  rc2Raw,
		"/lab/latest/manifest.json.sig.json":         rc1Sig, // signature for rc1, manifest rc2
		"/lab/1.0.0-rc2/pve-zfs-monitor-linux-amd64": []byte{0x7f, 'E', 'L', 'F', '2'},
	}

	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	target := filepath.Join(dir, "pve-zfs-monitor")
	_ = os.WriteFile(target, binRC1, 0o755)
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))
	if err := WritePersistedConfig(PersistedConfig{
		ReleaseBase: "https://releases.example.test",
		Channel:     ChannelLab,
	}, writePubKeyFile(t, pub)); err != nil {
		t.Fatal(err)
	}

	up := Updater{
		Client:           client,
		PublicKey:        pub,
		Current:          Info{Version: "1.0.0-rc1", Channel: "lab"},
		Channel:          ChannelLab,
		LockPath:         filepath.Join(dir, "install.lock"),
		InstallOpts:      InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ExecutableGOOS:   "linux",
		ExecutableGOARCH: "amd64",
	}
	_, _, _, err := up.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected signature mismatch failure")
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(binRC1) {
		t.Fatal("installed binary must not change on metadata mismatch")
	}
	_ = rc1Raw
}

func writeVersionBundle(t *testing.T, dir string, pub ed25519.PublicKey, version string, bin []byte) error {
	t.Helper()
	name := "pve-zfs-monitor-linux-amd64"
	_ = os.WriteFile(filepath.Join(dir, name), bin, 0o644)
	root := filepath.Clean(filepath.Join("..", ".."))
	asm := filepath.Join(root, "release", "signing", "assemble-manifest.py")
	manifestPath := filepath.Join(dir, "manifest.json")
	cmd := exec.Command("python3", asm,
		"--channel", "lab",
		"--version", version,
		"--commit", "deadbeef",
		"--release-base", "https://releases.example.test",
		"--artifact-dir", dir,
		"--output", manifestPath,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("assemble: %v\n%s", err, out)
	}
	raw, _ := os.ReadFile(manifestPath)
	_ = os.WriteFile(filepath.Join(dir, "manifest.json.sig.json"), signManifestJSON(t, raw), 0o644)
	return nil
}
