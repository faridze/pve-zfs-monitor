package release

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLabActionsPublishRC1ToRC2FromUnsignedArtifactDir(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))
	site := t.TempDir()

	// Existing public site with rc1.
	_ = os.MkdirAll(filepath.Join(site, "scripts"), 0o755)
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("<html></html>"), 0o644)
	_ = os.WriteFile(filepath.Join(site, ".nojekyll"), nil, 0o644)
	pubPath := filepath.Join(site, "scripts", "trusted-signing-key.pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)
	_ = os.WriteFile(filepath.Join(site, "scripts", "install.sh"), []byte("#!/bin/sh\n"), 0o644)
	_ = os.WriteFile(filepath.Join(site, "scripts", "install.sh.sha256"), []byte("abc  install.sh\n"), 0o644)

	rc1Dir := filepath.Join(site, "lab", "1.0.0-rc1")
	_ = os.MkdirAll(rc1Dir, 0o755)
	bin1 := []byte{0x7f, 'E', 'L', 'F', '1'}
	_ = writeVersionBundle(t, rc1Dir, pub, "1.0.0-rc1", bin1)
	_ = os.WriteFile(filepath.Join(rc1Dir, "pve-zfs-monitor-linux-arm64"), bin1, 0o644)
	_ = os.WriteFile(filepath.Join(site, "lab", ".published-versions"), []byte("1.0.0-rc1\n"), 0o644)
	latest := filepath.Join(site, "lab", "latest")
	_ = os.MkdirAll(latest, 0o755)
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json"), filepath.Join(latest, "manifest.json"))
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json.sig.json"), filepath.Join(latest, "manifest.json.sig.json"))

	verifyPres := filepath.Join(root, "release", "signing", "verify-pages-site-preservation.sh")
	if out, err := exec.Command("bash", verifyPres, site).CombinedOutput(); err != nil {
		t.Fatalf("preservation pre-check: %v\n%s", err, out)
	}

	// Simulate download-artifact destination (basename "unsigned", not the semver).
	unsigned := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsigned, 0o755)
	bin2 := []byte{0x7f, 'E', 'L', 'F', '2'}
	_ = writeVersionBundle(t, unsigned, pub, "1.0.0-rc2", bin2)
	_ = os.WriteFile(filepath.Join(unsigned, "pve-zfs-monitor-linux-arm64"), bin2, 0o644)

	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	cmd := exec.Command("bash", merge, site, unsigned, pubPath, root, "1.0.0-rc2")
	cmd.Env = append(os.Environ(), "EXPECTED_LAB_VERSION=1.0.0-rc2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}

	rc2Dir := filepath.Join(site, "lab", "1.0.0-rc2")
	for _, f := range []string{"manifest.json", "manifest.json.sig.json", "pve-zfs-monitor-linux-amd64", "pve-zfs-monitor-linux-arm64"} {
		if _, err := os.Stat(filepath.Join(rc2Dir, f)); err != nil {
			t.Fatalf("missing lab/1.0.0-rc2/%s: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(site, "lab", "unsigned")); err == nil {
		t.Fatal("lab/unsigned must not be created")
	}
	latestRaw, _ := os.ReadFile(filepath.Join(site, "lab", "latest", "manifest.json"))
	if !strings.Contains(string(latestRaw), "1.0.0-rc2") {
		t.Fatalf("lab/latest must point to rc2, got %s", latestRaw)
	}
	if out2, err := exec.Command("bash", verifyPres, site).CombinedOutput(); err != nil {
		t.Fatalf("preservation post-check: %v\n%s", err, out2)
	}
}

func TestCheckLabVersionNotPublishedFailClosed(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "release", "signing", "check-lab-version-not-published.sh")

	t.Run("404 ok", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.NotFound(w, r)
		}))
		defer srv.Close()
		cmd := exec.Command("bash", script, srv.URL, "9.9.9-test")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("expected success on 404: %v\n%s", err, out)
		}
	})

	t.Run("200 rejects", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("{}"))
		}))
		defer srv.Close()
		cmd := exec.Command("bash", script, srv.URL, "1.0.0-rc1")
		_, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected failure when version already published")
		}
	})

	t.Run("500 aborts", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		}))
		defer srv.Close()
		cmd := exec.Command("bash", script, srv.URL, "1.0.0-rc1")
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("expected failure on HTTP 500")
		}
		if !strings.Contains(string(out), "unexpected HTTP 500") {
			t.Fatalf("unexpected output: %s", out)
		}
	})
}

func TestMergeRejectsManifestVersionMismatch(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))
	site := t.TempDir()
	pubPath := filepath.Join(t.TempDir(), "pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)

	unsigned := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsigned, 0o755)
	_ = writeVersionBundle(t, unsigned, pub, "1.0.0-rc2", []byte{1})
	_ = os.WriteFile(filepath.Join(unsigned, "pve-zfs-monitor-linux-arm64"), []byte{1}, 0o644)

	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	out, err := exec.Command("bash", merge, site, unsigned, pubPath, root, "1.0.0-rc9").CombinedOutput()
	if err == nil {
		t.Fatalf("expected version mismatch failure, out=%s", out)
	}
	if !strings.Contains(string(out), "does not match expected") {
		t.Fatalf("unexpected output: %s", out)
	}
}
