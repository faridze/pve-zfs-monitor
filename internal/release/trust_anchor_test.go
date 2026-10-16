package release

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallReleaseRejectsTrustKeyMismatch(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))

	pubA, _ := testSigningKeypair()
	pubB, privB, _ := ed25519.GenerateKey(rand.Reader)
	keyB := filepath.Join(dir, "b.b64")
	_ = os.WriteFile(keyB, []byte(base64.StdEncoding.EncodeToString(pubB)), 0o644)

	base := "https://releases.example.test"
	if err := WritePersistedConfig(PersistedConfig{ReleaseBase: base, Channel: ChannelLab}, writePubKeyFile(t, pubA)); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "release.conf"))

	target := filepath.Join(dir, "pve-zfs-monitor")
	manifestPath, sigPath, binPath := writeSignedBundleForKey(t, pubB, privB, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F'})

	err := InstallRelease(InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pubB,
		Opts:               InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:        base,
		Channel:            ChannelLab,
		ExpectedVersion:    "1.0.0-rc2",
		SourceTrustKeyFile: keyB,
	})
	if !errors.Is(err, ErrTrustKeyMismatch) {
		t.Fatalf("expected ErrTrustKeyMismatch, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "release.conf"))
	if string(after) != string(before) {
		t.Fatal("config changed on trust key mismatch")
	}
}

func TestInstallReleaseAllowsTrustKeyRotationFlag(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))

	pubA, _ := testSigningKeypair()
	pubB, privB, _ := ed25519.GenerateKey(rand.Reader)
	keyB := filepath.Join(dir, "b.b64")
	_ = os.WriteFile(keyB, []byte(base64.StdEncoding.EncodeToString(pubB)), 0o644)

	base := "https://releases.example.test"
	if err := WritePersistedConfig(PersistedConfig{ReleaseBase: base, Channel: ChannelLab}, writePubKeyFile(t, pubA)); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(dir, "pve-zfs-monitor")
	manifestPath, sigPath, binPath := writeSignedBundleForKey(t, pubB, privB, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F'})

	if err := InstallRelease(InstallReleaseRequest{
		ManifestPath:          manifestPath,
		SignaturePath:         sigPath,
		ArtifactPath:          binPath,
		PublicKey:             pubB,
		Opts:                  InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:           base,
		Channel:               ChannelLab,
		ExpectedVersion:       "1.0.0-rc2",
		SourceTrustKeyFile:    keyB,
		AllowTrustKeyRotation: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "trusted-signing-key.pub.b64"))
	if string(got) != base64.StdEncoding.EncodeToString(pubB) {
		t.Fatal("rotated trust key not persisted")
	}
}

func TestInstallReleaseKeepsConfigWhenPostRenameSyncFails(t *testing.T) {
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
	newBase := "https://new.example.test"
	before, _ := os.ReadFile(filepath.Join(dir, "release.conf"))

	target := filepath.Join(dir, "pve-zfs-monitor")
	manifestPath, sigPath, binPath := writeSignedBundle(t, pub, "1.0.0-rc2", []byte{0x7f, 'E', 'L', 'F'})

	syncTargetAfterActivationHook = func(string) error { return errors.New("simulated fsync failure") }
	defer func() { syncTargetAfterActivationHook = nil }()

	err := InstallRelease(InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pub,
		Opts:               InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:        newBase,
		Channel:            ChannelLab,
		ExpectedVersion:    "1.0.0-rc2",
		SourceTrustKeyFile: keyPath,
	})
	if !errors.Is(err, ErrActivationDurability) {
		t.Fatalf("expected durability error, got %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "release.conf"))
	if string(after) == string(before) {
		t.Fatal("config incorrectly rolled back after successful activation")
	}
	gotBin, _ := os.ReadFile(target)
	if len(gotBin) == 0 {
		t.Fatal("binary not activated")
	}
}

func writeSignedBundleForKey(t *testing.T, pub ed25519.PublicKey, priv ed25519.PrivateKey, version string, bin []byte) (manifestPath, sigPath, binPath string) {
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
	sig := ed25519.Sign(priv, raw)
	env, _ := json.Marshal(SignatureEnvelope{
		Algorithm:   SignatureAlgorithmEd25519,
		PublicKeyID: "test-only",
		Signature:   base64.StdEncoding.EncodeToString(sig),
	})
	_ = os.WriteFile(sigPath, env, 0o644)
	_ = os.WriteFile(binPath, bin, 0o644)
	_ = pub
	return manifestPath, sigPath, binPath
}
