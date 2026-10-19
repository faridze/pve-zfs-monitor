package release

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestHermeticBootstrapUpgradeRollbackLifecycle exercises the real curl/bash
// bootstrap, signed Go updater and rollback using only scratch paths, a local
// TLS server and a generated throwaway Ed25519 key. No production host,
// release site, physical disk or trusted signing key is accessed.
func TestHermeticBootstrapUpgradeRollbackLifecycle(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("GitHub-hosted Linux AMD64 runner acceptance")
	}
	for _, name := range []string{"bash", "curl", "python3", "openssl"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s unavailable: %v", name, err)
		}
	}

	pub, _ := testSigningKeypair()
	rc1 := []byte{0x7f, 'E', 'L', 'F', 'r', 'c', '1'}
	rc2 := []byte{0x7f, 'E', 'L', 'F', 'r', 'c', '2'}
	var corrupt atomic.Bool

	var raw1, sig1, raw2, sig2 []byte
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "GET only", http.StatusMethodNotAllowed)
			return
		}
		var b []byte
		switch r.URL.Path {
		case "/releases/lab/1.0.0-rc1/manifest.json":
			b = raw1
		case "/releases/lab/1.0.0-rc1/manifest.json.sig.json":
			b = sig1
		case "/releases/lab/1.0.0-rc1/pve-zfs-monitor-linux-amd64":
			b = rc1
		case "/releases/lab/latest/manifest.json":
			b = raw2
		case "/releases/lab/latest/manifest.json.sig.json":
			b = sig2
		case "/releases/lab/1.0.0-rc2/pve-zfs-monitor-linux-amd64":
			b = rc2
			if corrupt.Load() {
				b = []byte("tampered-artifact")
			}
		default:
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()
	base := srv.URL + "/releases"
	_, raw1, sig1 = buildLabManifestAtVersion(t, base, "1.0.0-rc1", rc1)
	_, raw2, sig2 = buildLabManifestAtVersion(t, base, "1.0.0-rc2", rc2)

	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "scripts", "install.sh")
	dir := t.TempDir()
	target := filepath.Join(dir, "bin", "pve-zfs-monitor")
	previous := target + RollbackSuffix
	confDir := filepath.Join(dir, "etc")
	lock := filepath.Join(dir, "run", "install.lock")
	pubFile := filepath.Join(dir, "trust.pub.b64")
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pubFile, []byte(testPubB64), 0o600); err != nil {
		t.Fatal(err)
	}

	// Environment overrides apply only in hermetic test mode; production
	// paths and private signing keys are never used.
	cmd := exec.Command("bash", script,
		"--channel", "lab", "--version", "1.0.0-rc1",
		"--release-base", base, "--trust-key-file", pubFile)
	cmd.Env = append(os.Environ(),
		"PVE_ZFS_MONITOR_HERMETIC_TEST=1",
		"PVE_ZFS_MONITOR_CURL_INSECURE=1",
		"INSTALLER_SHA256="+installerScriptSHA256(script),
		"PVE_ZFS_MONITOR_INSTALL_TARGET="+target,
		"PVE_ZFS_MONITOR_CONFIG_DIR="+confDir,
		"PVE_ZFS_MONITOR_LOCK_PATH="+lock,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("rc1 bootstrap failed: %v\n%s", err, output)
	}
	assertBinary := func(expected []byte, step string) {
		t.Helper()
		got, err := os.ReadFile(target)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("%s: binary mismatch: %v", step, err)
		}
	}
	assertBinary(rc1, "bootstrap")
	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Fatalf("bootstrap must not create prior version, stat=%v", err)
	}

	t.Setenv("PVE_ZFS_MONITOR_HERMETIC_TEST", "1")
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", confDir)
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", lock)
	t.Setenv("PVE_ZFS_MONITOR_RELEASE_BASE", "")
	cfg, err := LoadPersistedConfig()
	if err != nil {
		t.Fatalf("bootstrap config: %v", err)
	}
	if cfg.ReleaseBase != base || cfg.Channel != ChannelLab {
		t.Fatalf("wrong saved bootstrap config: %+v", cfg)
	}
	beforeCfg, err := os.ReadFile(filepath.Join(confDir, "release.conf"))
	if err != nil {
		t.Fatal(err)
	}
	beforeKey, err := os.ReadFile(filepath.Join(confDir, "trusted-signing-key.pub.b64"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(beforeKey), []byte(testPubB64)) {
		t.Fatal("bootstrap persisted incorrect public key")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	up := Updater{
		Secure:           SecureHTTPClient{Client: srv.Client()},
		PublicKey:        pub,
		Current:          Info{Version: "1.0.0-rc1", Channel: "lab"},
		Channel:          ChannelLab,
		LockPath:         lock,
		InstallOpts:      InstallOptions{TargetPath: target, RollbackPath: previous, ExpectedName: "pve-zfs-monitor"},
		ExecutableGOOS:   "linux",
		ExecutableGOARCH: "amd64",
	}
	plan, _, _, err := up.CheckForUpdate(ctx)
	if err != nil || plan.Available.Version != "1.0.0-rc2" {
		t.Fatalf("expected signed rc2 update: plan=%v err=%v", plan != nil, err)
	}
	if err := up.ApplyUpdate(ctx, plan, true); err != nil {
		t.Fatalf("dry-run verification failed: %v", err)
	}
	assertBinary(rc1, "dry-run must not replace rc1")
	if _, err := os.Stat(previous); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not create rollback copy, stat=%v", err)
	}

	corrupt.Store(true)
	if err := up.ApplyUpdate(ctx, plan, false); err == nil {
		t.Fatal("corrupted artifact was accepted")
	}
	corrupt.Store(false)
	assertBinary(rc1, "tampered artifact must not replace rc1")

	if err := up.ApplyUpdate(ctx, plan, false); err != nil {
		t.Fatalf("signed rc2 upgrade failed: %v", err)
	}
	assertBinary(rc2, "upgrade")
	prev, err := os.ReadFile(previous)
	if err != nil || !bytes.Equal(prev, rc1) {
		t.Fatalf("upgrade did not retain rc1 rollback binary: %v", err)
	}
	up.Current = Info{Version: "1.0.0-rc2", Channel: "lab"}
	if _, _, _, err = up.CheckForUpdate(ctx); err != ErrUpToDate {
		t.Fatalf("expected already up to date: %v", err)
	}
	if err := Rollback(up.InstallOpts); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}
	assertBinary(rc1, "rollback")
	afterCfg, err := os.ReadFile(filepath.Join(confDir, "release.conf"))
	if err != nil {
		t.Fatal(err)
	}
	afterKey, err := os.ReadFile(filepath.Join(confDir, "trusted-signing-key.pub.b64"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(beforeCfg, afterCfg) || !bytes.Equal(beforeKey, afterKey) {
		t.Fatal("upgrade/rollback changed the persisted release trust settings")
	}
	t.Logf("%s", strings.Join([]string{
		"hermetic bootstrap rc1 PASS",
		"signed update check PASS",
		"verified dry-run PASS",
		"tampered artifact rejection PASS",
		"signed upgrade rc2 PASS",
		"rollback rc1 PASS",
		"config/trust preservation PASS",
	}, "; "))
}
