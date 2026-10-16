package release

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestSecureConfigAllows0644NotWritable(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	pub, _ := testSigningKeypair()
	keyPath := filepath.Join(dir, "admin-key.b64")
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WritePersistedConfig(PersistedConfig{
		ReleaseBase: "https://releases.example.test",
		Channel:     ChannelLab,
	}, keyPath); err != nil {
		t.Fatal(err)
	}
	confPath := filepath.Join(dir, "release.conf")
	fi, err := os.Stat(confPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o644 != 0o644 {
		t.Fatalf("expected 0644-ish mode, got %o", fi.Mode().Perm())
	}
	if err := assertSecureConfigPath(confPath); err != nil {
		t.Fatalf("0644 config should pass security checks: %v", err)
	}
	os.Unsetenv("PVE_ZFS_MONITOR_RELEASE_BASE")
	os.Unsetenv("PVE_ZFS_MONITOR_TRUST_KEY_FILE")
	if _, err := LoadPersistedConfig(); err != nil {
		t.Fatalf("LoadPersistedConfig: %v", err)
	}
}

func TestSecureConfigRejectsWorldWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "release.conf")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := assertSecureConfigPath(path); err == nil {
		t.Fatal("expected rejection of world-writable config")
	}
}

func TestSecureConfigRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "release.conf.real")
	if err := os.WriteFile(real, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "release.conf")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := assertSecureConfigPath(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
