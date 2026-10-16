package release

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallReleaseRestoresConfigOnBinaryFailure(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))

	pub, _ := testSigningKeypair()
	oldBase := "https://old.example.test/releases"
	if err := WritePersistedConfig(PersistedConfig{
		ReleaseBase: oldBase,
		Channel:     ChannelLab,
	}, writePubKeyFile(t, pub)); err != nil {
		t.Fatal(err)
	}
	oldCfg, err := os.ReadFile(filepath.Join(dir, "release.conf"))
	if err != nil {
		t.Fatal(err)
	}

	binDir := filepath.Join(dir, "bindir")
	target := filepath.Join(binDir, "pve-zfs-monitor")
	_ = os.MkdirAll(binDir, 0o755)
	_ = os.WriteFile(target, []byte("v1"), 0o755)

	manifestPath, sigPath, binPath := writeSignedBundle(t, pub, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F', '2'})
	newKey := filepath.Join(dir, "new-key.b64")
	_ = os.WriteFile(newKey, []byte(testPubB64), 0o644)

	_ = os.Chmod(binDir, 0o500)

	err = InstallRelease(InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pub,
		Opts:               InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:        "https://new.example.test/releases",
		Channel:            ChannelLab,
		ExpectedVersion:    "1.0.0-rc2",
		SourceTrustKeyFile: newKey,
	})
	_ = os.Chmod(binDir, 0o755)
	if err == nil {
		t.Fatal("expected binary activation failure")
	}
	gotCfg, err := os.ReadFile(filepath.Join(dir, "release.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if string(gotCfg) != string(oldCfg) {
		t.Fatalf("config not restored:\nold %s\nnew %s", oldCfg, gotCfg)
	}
	gotBin, _ := os.ReadFile(target)
	if string(gotBin) != "v1" {
		t.Fatalf("binary changed: %q", gotBin)
	}
}

func TestInstallReleaseSkipsConfigRewriteOnSameChannelBase(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))

	pub, _ := testSigningKeypair()
	base := "https://releases.example.test"
	keyPath := writePubKeyFile(t, pub)
	if err := WritePersistedConfig(PersistedConfig{ReleaseBase: base, Channel: ChannelLab}, keyPath); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "release.conf"))

	target := filepath.Join(dir, "pve-zfs-monitor")
	manifestPath, sigPath, binPath := writeSignedBundle(t, pub, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F'})

	if err := InstallRelease(InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pub,
		Opts:               InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:        base,
		Channel:            ChannelLab,
		ExpectedVersion:    "1.0.0-rc2",
		SourceTrustKeyFile: keyPath,
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "release.conf"))
	if string(after) != string(before) {
		t.Fatal("config rewritten despite matching channel and release base")
	}
}

func writeSignedBundle(t *testing.T, pub ed25519.PublicKey, version string, bin []byte) (manifestPath, sigPath, binPath string) {
	t.Helper()
	dir := t.TempDir()
	sum := sha256.Sum256(bin)
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       version,
		Channel:       ChannelLab,
		Artifacts: []Artifact{{
			Filename: "pve-zfs-monitor-linux-amd64",
			GOOS:     "linux",
			GOARCH:   "amd64",
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(bin)),
		}},
	}
	raw, _ := json.Marshal(manifest)
	manifestPath = filepath.Join(dir, "manifest.json")
	sigPath = filepath.Join(dir, "manifest.json.sig.json")
	binPath = filepath.Join(dir, "bin")
	_ = os.WriteFile(manifestPath, raw, 0o644)
	_ = os.WriteFile(sigPath, signManifestJSON(t, raw), 0o644)
	_ = os.WriteFile(binPath, bin, 0o644)
	return manifestPath, sigPath, binPath
}
