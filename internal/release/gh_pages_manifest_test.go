package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitHubPagesManifestURLContract(t *testing.T) {
	path := filepath.Clean(filepath.Join("..", "..", "dist", "gh-pages-staging", "1.0.0-rc1", "manifest.json"))
	if _, err := os.Stat(path); err != nil {
		t.Skip("staging manifest not built in this workspace")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := ParseManifest(raw)
	if err != nil {
		t.Fatal(err)
	}
	base := "https://faridze.github.io/pve-zfs-monitor"
	if m.Version != "1.0.0-rc1" || m.Channel != ChannelLab {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if m.Commit != "b2245e8a28bc393ef31ab343ba2c507c67e1508a" {
		t.Fatalf("unexpected commit %q", m.Commit)
	}
	for _, art := range m.Artifacts {
		if !strings.HasPrefix(art.URL, base+"/lab/1.0.0-rc1/") {
			t.Fatalf("bad url %q", art.URL)
		}
		if art.Size <= 0 || art.SHA256 == "" {
			t.Fatal("missing size/sha256")
		}
	}
	urls, err := ResolveChannelURLs(ChannelLab, "")
	if err != nil && strings.Contains(err.Error(), "not configured") {
		t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
		dir := t.TempDir()
		t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
		pub, _ := testSigningKeypair()
		_ = WritePersistedConfig(PersistedConfig{ReleaseBase: base, Channel: ChannelLab}, writePubKeyFile(t, pub))
		urls, err = ResolveChannelURLs(ChannelLab, "")
	}
	if err != nil {
		t.Fatal(err)
	}
	if urls.ManifestURL != base+"/lab/latest/manifest.json" {
		t.Fatalf("latest manifest url: %s", urls.ManifestURL)
	}
	if urls.SignatureURL != base+"/lab/latest/manifest.json.sig.json" {
		t.Fatalf("latest sig url: %s", urls.SignatureURL)
	}
}
