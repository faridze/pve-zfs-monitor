package release

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

func TestPersistedConfigEnablesUpdateWithoutEnv(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
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
	os.Unsetenv("PVE_ZFS_MONITOR_RELEASE_BASE")
	os.Unsetenv("PVE_ZFS_MONITOR_TRUST_KEY_FILE")
	base, err := ResolveReleaseBase()
	if err != nil || base != "https://releases.example.test" {
		t.Fatalf("base=%q err=%v", base, err)
	}
	got, err := LoadTrustedPublicKey()
	if err != nil || len(got) == 0 {
		t.Fatalf("key load: %v", err)
	}
}
