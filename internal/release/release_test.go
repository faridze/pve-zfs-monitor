package release

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type staticHTTP map[string][]byte

func (s staticHTTP) Get(ctx context.Context, url string) ([]byte, error) {
	for k, v := range s {
		if strings.HasSuffix(url, k) || url == k {
			return v, nil
		}
	}
	return nil, fmt.Errorf("not found: %s", url)
}

func TestVersionMetadataDevelopment(t *testing.T) {
	info := CurrentInfo()
	if info.Development {
		// default dev build
	}
	if info.TrustedRelease {
		t.Fatal("dev build must not be trusted")
	}
}

func TestPlanUpdateUpToDate(t *testing.T) {
	_, err := PlanUpdate(Info{Version: "1.0.0-rc2", Channel: "lab"}, &Manifest{
		Version: "1.0.0-rc2", Channel: ChannelLab,
		Artifacts: []Artifact{{Filename: "b", GOOS: "linux", GOARCH: "amd64", SHA256: "00", Size: 1, URL: "https://x.example/b"}},
	}, "linux", "amd64", false)
	if !errors.Is(err, ErrUpToDate) {
		t.Fatalf("expected ErrUpToDate, got %v", err)
	}
}

func TestCompareSemverAndDowngrade(t *testing.T) {
	if c, err := CompareSemver("1.2.3", "1.2.4"); err != nil || c >= 0 {
		t.Fatalf("compare %d err=%v", c, err)
	}
	_, err := PlanUpdate(Info{Version: "2.0.0", Channel: "stable"}, &Manifest{
		Version: "1.0.0", Channel: ChannelStable,
		Artifacts: []Artifact{{Filename: "b", GOOS: "linux", GOARCH: "amd64", SHA256: "00", Size: 1, URL: "https://x.example/b"}},
	}, "linux", "amd64", false)
	if err == nil || !strings.Contains(err.Error(), "downgrade") {
		t.Fatalf("expected downgrade rejection, got %v", err)
	}
}

func TestInvalidManifestAndSignature(t *testing.T) {
	pub, _ := testSigningKeypair()
	raw := []byte(`{"schema_version":1}`)
	if err := VerifyManifestSignature(raw, &SignatureEnvelope{Algorithm: "ed25519", Signature: "AAAA"}, pub); err == nil {
		t.Fatal("expected bad signature")
	}
	if _, err := ParseManifest([]byte(`{}`)); err == nil {
		t.Fatal("expected invalid manifest")
	}
}

func TestInstallAtomicAndRollback(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "lock"))
	target := filepath.Join(dir, "pve-zfs-monitor")
	opts := InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"}
	staged := filepath.Join(dir, "newbin")
	if err := os.WriteFile(staged, []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicInstall(staged, opts); err != nil {
		t.Fatal(err)
	}
	if err := ValidateInstallTarget(target); err != nil {
		t.Fatal(err)
	}
	staged2 := filepath.Join(dir, "newbin2")
	if err := os.WriteFile(staged2, []byte("#!/bin/sh\necho v2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicInstall(staged2, opts); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(opts); err != nil {
		t.Fatal(err)
	}
}

func TestRefuseUnexpectedSymlink(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "lock"))
	link := filepath.Join(dir, "pve-zfs-monitor")
	if err := os.Symlink("/bin/sh", link); err != nil {
		t.Fatal(err)
	}
	opts := InstallOptions{TargetPath: link, RollbackPath: link + RollbackSuffix, ExpectedName: "pve-zfs-monitor"}
	if err := ValidateInstallTarget(link); err == nil {
		t.Fatal("expected symlink rejection")
	}
	_ = opts
}

func TestUpdateFlowWithHTTPServer(t *testing.T) {
	pub, _ := testSigningKeypair()
	t.Setenv("PVE_ZFS_MONITOR_TRUST_KEY_FILE", writePubKeyFile(t, pub))
	t.Setenv("PVE_ZFS_MONITOR_RELEASE_BASE", "https://releases.example.test")
	bin := []byte{0x7f, 'E', 'L', 'F'}
	sum := sha256Hex(bin)
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       "1.0.0",
		Channel:       ChannelLab,
		Commit:        "abc",
		PublishedAt:   "2026-01-01T00:00:00Z",
		Artifacts: []Artifact{{
			Filename: "pve-zfs-monitor",
			GOOS:     "linux",
			GOARCH:   "amd64",
			SHA256:   sum,
			Size:     int64(len(bin)),
			URL:      "https://releases.example.test/artifact",
		}},
	}
	raw, _ := json.Marshal(manifest)
	sig := signManifestJSON(t, raw)
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "update.lock"))
	target := filepath.Join(dir, "pve-zfs-monitor")
	up := Updater{
		LockPath: filepath.Join(dir, "update.lock"),
		Client: staticHTTP{
			"/manifest.json":          raw,
			"/manifest.json.sig.json": sig,
			"/artifact":               bin,
		},
		PublicKey:        pub,
		Current:          Info{Version: "0.9.0", Channel: "lab"},
		Channel:          ChannelLab,
		InstallOpts:      InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ExecutableGOOS:   "linux",
		ExecutableGOARCH: "amd64",
	}
	// override URLs manually for static client
	urls := ChannelURLs{ManifestURL: "/manifest.json", SignatureURL: "/manifest.json.sig.json"}
	manifestParsed, _, _, err := FetchChannelManifest(context.Background(), up.Client, urls, pub)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanUpdate(up.Current, manifestParsed, "linux", "amd64", false)
	if err != nil {
		t.Fatal(err)
	}
	plan.Artifact.URL = "https://releases.example.test/artifact"
	if err := up.ApplyUpdate(context.Background(), plan, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestConcurrentUpdateLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	l1, err := AcquireUpdateLock(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = AcquireUpdateLock(path)
	if err == nil {
		t.Fatal("expected concurrent lock failure")
	}
	_ = l1.Release()
}

func TestArchitectureMismatch(t *testing.T) {
	m := &Manifest{
		Version: "1.0.0", Channel: ChannelStable,
		Artifacts: []Artifact{{Filename: "b", GOOS: "linux", GOARCH: "arm64", SHA256: "ab", Size: 2, URL: "u"}},
	}
	_, err := m.SelectArtifact("linux", "amd64")
	if err == nil {
		t.Fatal("expected arch mismatch")
	}
}

func TestChannelIsolation(t *testing.T) {
	_, err := PlanUpdate(Info{Version: "0.1.0", Channel: "stable"}, &Manifest{
		Version: "0.2.0", Channel: ChannelLab,
		Artifacts: []Artifact{{Filename: "b", GOOS: "linux", GOARCH: "amd64", SHA256: "00", Size: 1, URL: "u"}},
	}, "linux", "amd64", false)
	if err == nil {
		t.Fatal("expected channel mismatch")
	}
}

func TestOfflineUpdate(t *testing.T) {
	pub, _ := testSigningKeypair()
	up := Updater{
		Client:    staticHTTP{},
		PublicKey: pub,
		Current:   Info{Version: "0.1.0", Channel: "lab"},
		Channel:   ChannelLab,
	}
	_, _, _, err := up.CheckForUpdate(context.Background())
	if err == nil {
		t.Fatal("expected offline failure")
	}
}

func writePubKeyFile(t *testing.T, pub ed25519.PublicKey) string {
	path := filepath.Join(t.TempDir(), "pub.b64")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(pub)), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
