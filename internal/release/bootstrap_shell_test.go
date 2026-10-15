package release

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func runBashSyntaxCheck(script string) (string, error) {
	cmd := exec.Command("bash", "-n", script)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func goarchForTest() string {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return runtime.GOARCH
	default:
		return "amd64"
	}
}

func TestInstallScriptBootstrapFromEmptyDir(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	if _, err := exec.LookPath("openssl"); err != nil {
		t.Skip("openssl not available")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl not available")
	}

	_, priv := testSigningKeypair()
	goarch := goarchForTest()
	artifactName := "pve-zfs-monitor-linux-" + goarch
	bin := []byte{0x7f, 'E', 'L', 'F', 't', 'e', 's', 't'}
	sum := sha256.Sum256(bin)
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       "1.0.0-rc2",
		Channel:       ChannelLab,
		Commit:        "deadbeef",
		PublishedAt:   "2026-01-01T00:00:00Z",
		Artifacts: []Artifact{{
			Filename: artifactName,
			GOOS:     "linux",
			GOARCH:   goarch,
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(bin)),
		}},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	sig := signManifestJSON(t, raw)

	mux := http.NewServeMux()
	base := "/releases/lab/1.0.0-rc2"
	mux.HandleFunc(base+"/manifest.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write(raw)
	})
	mux.HandleFunc(base+"/manifest.json.sig.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write(sig)
	})
	mux.HandleFunc(base+"/"+artifactName, func(w http.ResponseWriter, r *http.Request) {
		w.Write(bin)
	})
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	work := t.TempDir()
	installDir := filepath.Join(work, "bootstrap-only")
	if err := os.Mkdir(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	scriptSrc := filepath.Join(root, "scripts", "install.sh")
	scriptDst := filepath.Join(installDir, "install.sh")
	data, err := os.ReadFile(scriptSrc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scriptDst, data, 0o755); err != nil {
		t.Fatal(err)
	}
	trustKey := filepath.Join(installDir, "trust.pub.b64")
	if err := os.WriteFile(trustKey, []byte(testPubB64), 0o644); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(work, "bin")
	confDir := filepath.Join(work, "etc")
	lockDir := filepath.Join(work, "lock")
	_ = os.MkdirAll(targetDir, 0o755)
	_ = os.MkdirAll(confDir, 0o755)
	_ = os.MkdirAll(lockDir, 0o755)

	releaseBase := strings.TrimSuffix(srv.URL, "/") + "/releases"
	env := []string{
		"PVE_ZFS_MONITOR_TEST_INSTALL=1",
		"PVE_ZFS_MONITOR_CURL_INSECURE=1",
		"PVE_ZFS_MONITOR_INSTALL_TARGET=" + filepath.Join(targetDir, "pve-zfs-monitor"),
		"PVE_ZFS_MONITOR_CONFIG_DIR=" + confDir,
		"PVE_ZFS_MONITOR_LOCK_PATH=" + filepath.Join(lockDir, "pve-zfs-monitor.lock"),
	}
	args := []string{scriptDst,
		"--channel", "lab",
		"--version", "1.0.0-rc2",
		"--release-base", releaseBase,
		"--trust-key-file", trustKey,
	}
	cmd := exec.Command("bash", args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, out)
	}
	target := filepath.Join(targetDir, "pve-zfs-monitor")
	got, err := os.ReadFile(target)
	if err != nil || string(got) != string(bin) {
		t.Fatalf("binary not installed: err=%v got=%q", err, got)
	}
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", confDir)
	cfg, err := LoadPersistedConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReleaseBase != releaseBase {
		t.Fatalf("release base %q", cfg.ReleaseBase)
	}
	_ = priv
}

func TestInstallScriptRejectsTamperedArtifact(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	pub, _ := testSigningKeypair()
	goarch := goarchForTest()
	artifactName := "pve-zfs-monitor-linux-" + goarch
	bin := []byte{0x7f, 'E', 'L', 'F'}
	sum := sha256.Sum256(bin)
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       "1.0.0-rc1",
		Channel:       ChannelLab,
		Artifacts: []Artifact{{
			Filename: artifactName,
			GOOS:     "linux",
			GOARCH:   goarch,
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(bin)),
		}},
	}
	raw, _ := json.Marshal(manifest)
	sig := signManifestJSON(t, raw)
	tampered := []byte("not-the-binary")

	mux := http.NewServeMux()
	base := "/releases/lab/1.0.0-rc1"
	mux.HandleFunc(base+"/manifest.json", func(w http.ResponseWriter, r *http.Request) { w.Write(raw) })
	mux.HandleFunc(base+"/manifest.json.sig.json", func(w http.ResponseWriter, r *http.Request) { w.Write(sig) })
	mux.HandleFunc(base+"/"+artifactName, func(w http.ResponseWriter, r *http.Request) { w.Write(tampered) })
	srv := httptest.NewTLSServer(mux)
	defer srv.Close()

	work := t.TempDir()
	installDir := filepath.Join(work, "only-installer")
	_ = os.Mkdir(installDir, 0o755)
	root := filepath.Clean(filepath.Join("..", ".."))
	scriptDst := filepath.Join(installDir, "install.sh")
	data, _ := os.ReadFile(filepath.Join(root, "scripts", "install.sh"))
	_ = os.WriteFile(scriptDst, data, 0o755)
	trustKey := filepath.Join(installDir, "trust.pub.b64")
	_ = os.WriteFile(trustKey, []byte(testPubB64), 0o644)

	targetDir := filepath.Join(work, "bin")
	confDir := filepath.Join(work, "etc")
	_ = os.MkdirAll(targetDir, 0o755)
	releaseBase := strings.TrimSuffix(srv.URL, "/") + "/releases"
	cmd := exec.Command("bash", scriptDst,
		"--channel", "lab", "--version", "1.0.0-rc1",
		"--release-base", releaseBase, "--trust-key-file", trustKey)
	cmd.Env = append(os.Environ(),
		"PVE_ZFS_MONITOR_TEST_INSTALL=1",
		"PVE_ZFS_MONITOR_CURL_INSECURE=1",
		"PVE_ZFS_MONITOR_INSTALL_TARGET="+filepath.Join(targetDir, "pve-zfs-monitor"),
		"PVE_ZFS_MONITOR_CONFIG_DIR="+confDir,
		"PVE_ZFS_MONITOR_LOCK_PATH="+filepath.Join(work, "lock"),
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected failure, out=%s", out)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "pve-zfs-monitor")); err == nil {
		t.Fatal("tampered artifact must not be installed")
	}
	_ = pub
}

func TestInstallReleaseConfigFailureSkipsBinary(t *testing.T) {
	t.Setenv("PVE_ZFS_MONITOR_TEST_INSTALL", "1")
	dir := t.TempDir()
	t.Setenv("PVE_ZFS_MONITOR_LOCK_PATH", filepath.Join(dir, "lock"))
	pub, _ := testSigningKeypair()
	manifestPath := filepath.Join(dir, "manifest.json")
	sigPath := filepath.Join(dir, "manifest.json.sig.json")
	binPath := filepath.Join(dir, "bin")
	binData := []byte{0x7f, 'E', 'L', 'F'}
	sum := sha256.Sum256(binData)
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       "1.0.0",
		Channel:       ChannelLab,
		Artifacts: []Artifact{{
			Filename: "pve-zfs-monitor-linux-amd64",
			GOOS:     "linux",
			GOARCH:   "amd64",
			SHA256:   hex.EncodeToString(sum[:]),
			Size:     int64(len(binData)),
		}},
	}
	raw, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sigPath, signManifestJSON(t, raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binPath, binData, 0o644); err != nil {
		t.Fatal(err)
	}
	// Config dir is a file — WritePersistedConfig must fail before binary install.
	badConf := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(badConf, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PVE_ZFS_MONITOR_CONFIG_DIR", badConf)
	target := filepath.Join(dir, "target", "pve-zfs-monitor")
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	keyFile := filepath.Join(dir, "key.b64")
	_ = os.WriteFile(keyFile, []byte(testPubB64), 0o644)

	err := InstallRelease(InstallReleaseRequest{
		ManifestPath:       manifestPath,
		SignaturePath:      sigPath,
		ArtifactPath:       binPath,
		PublicKey:          pub,
		Opts:               InstallOptions{TargetPath: target, RollbackPath: target + RollbackSuffix, ExpectedName: "pve-zfs-monitor"},
		ReleaseBase:        "https://example.test/releases",
		Channel:            ChannelLab,
		ExpectedVersion:    "1.0.0",
		SourceTrustKeyFile: keyFile,
	})
	if err == nil {
		t.Fatal("expected config write failure")
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("binary must not be installed when config persistence fails")
	}
}
