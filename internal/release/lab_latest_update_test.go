package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabLatestRC1ToRC2UpdateFlow(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	pub, _ := testSigningKeypair()
	base := "https://releases.example.test"

	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", dir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "install.lock"))
	t.Setenv("PVE_ZFS_MONITOR_TRUST_KEY_FILE", writePubKeyFile(t, pub))
	if err := WritePersistedConfig(PersistedConfig{
		ReleaseBase: base,
		Channel:     ChannelLab,
	}, writePubKeyFile(t, pub)); err != nil {
		t.Fatal(err)
	}

	binRC1 := []byte{0x7f, 'E', 'L', 'F', 'r', 'c', '1'}
	binRC2 := []byte{0x7f, 'E', 'L', 'F', 'r', 'c', '2'}
	target := filepath.Join(dir, "pve-zfs-monitor")
	_ = os.WriteFile(target, binRC1, 0o755)

	_, rc2Raw, rc2Sig := buildLabManifestAtVersion(t, base, "1.0.0-rc2", binRC2)

	client := staticHTTP{
		"/lab/latest/manifest.json":                  rc2Raw,
		"/lab/latest/manifest.json.sig.json":         rc2Sig,
		"/lab/1.0.0-rc2/pve-zfs-monitor-linux-amd64": binRC2,
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

	plan, _, _, err := up.CheckForUpdate(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Available.Version != "1.0.0-rc2" {
		t.Fatalf("expected rc2, got %s", plan.Available.Version)
	}
	if err := up.ApplyUpdate(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(binRC2) {
		t.Fatalf("binary not updated: %q", got)
	}

	up.Current = Info{Version: "1.0.0-rc2", Channel: "lab"}
	_, _, _, err = up.CheckForUpdate(context.Background())
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("expected up to date after rc2 install, got %v", err)
	}
}

func buildLabManifestAtVersion(t *testing.T, base, version string, bin []byte) (*Manifest, []byte, []byte) {
	t.Helper()
	sum := sha256.Sum256(bin)
	name := "pve-zfs-monitor-linux-amd64"
	m := Manifest{
		SchemaVersion: 1,
		Version:       version,
		Channel:       ChannelLab,
		Commit:        "deadbeef",
		PublishedAt:   "2026-01-01T00:00:00Z",
		Artifacts: []Artifact{{
			Filename: name,
			GOOS:     "linux",
			GOARCH:   "amd64",
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(bin)),
			URL:      fmt.Sprintf("%s/lab/%s/%s", strings.TrimRight(base, "/"), version, name),
		}},
	}
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	return &m, raw, signManifestJSON(t, raw)
}
