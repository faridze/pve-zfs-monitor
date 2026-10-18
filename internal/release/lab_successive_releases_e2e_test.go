package release

import (
	"crypto/ed25519"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSuccessiveReleasesRC1RC2RC3LiveBaselineUnchangedGhPages(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))

	ghPagesFrozen := t.TempDir()
	deployed := t.TempDir()
	buildPublicRC1Site(t, pub, ghPagesFrozen, deployed)

	srv := httptest.NewServer(http.FileServer(http.Dir(deployed)))
	defer srv.Close()

	prepare := filepath.Join(root, "release", "signing", "prepare-pages-site-baseline.sh")
	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	materialize := filepath.Join(root, "release", "signing", "materialize-public-pages-site.sh")
	writeManifest := filepath.Join(root, "release", "signing", "write-pages-deploy-manifest.sh")
	verifyInv := filepath.Join(root, "release", "signing", "verify-pages-deploy-inventory.sh")

	// Migration release rc2: bootstrap from frozen gh-pages while live has no deploy inventory.
	workRC2 := t.TempDir()
	cmd := exec.Command("bash", prepare, srv.URL, workRC2)
	cmd.Env = append(os.Environ(),
		"PAGES_ALLOW_GH_PAGES_BOOTSTRAP=1",
		"PAGES_BOOTSTRAP_SOURCE_DIR="+ghPagesFrozen,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare rc2 baseline: %v\n%s", err, out)
	}
	pubPath := filepath.Join(workRC2, "scripts", "trusted-signing-key.pub.b64")
	unsignedRC2 := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsignedRC2, 0o755)
	bin2 := []byte{0x7f, 'E', 'L', 'F', '2'}
	_ = writeVersionBundle(t, unsignedRC2, pub, "1.0.0-rc2", bin2)
	_ = os.WriteFile(filepath.Join(unsignedRC2, "pve-zfs-monitor-linux-arm64"), bin2, 0o644)
	if out, err := exec.Command("bash", merge, workRC2, unsignedRC2, pubPath, root, "1.0.0-rc2").CombinedOutput(); err != nil {
		t.Fatalf("merge rc2: %v\n%s", err, out)
	}
	deployRC2 := t.TempDir()
	if out, err := exec.Command("bash", materialize, workRC2, deployRC2).CombinedOutput(); err != nil {
		t.Fatalf("materialize rc2: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", writeManifest, deployRC2).CombinedOutput(); err != nil {
		t.Fatalf("write manifest rc2: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", verifyInv, deployRC2).CombinedOutput(); err != nil {
		t.Fatalf("verify inventory rc2: %v\n%s", err, out)
	}
	assertNoForbiddenDeploymentPaths(t, deployRC2)
	resetDir(t, deployed, deployRC2)
	assertInventoryPathsFetchable(t, srv.URL, deployRC2)

	// gh-pages frozen copy must still only contain rc1 (no rc2 directory added).
	if _, err := os.Stat(filepath.Join(ghPagesFrozen, "lab", "1.0.0-rc2")); err == nil {
		t.Fatal("frozen gh-pages must not gain rc2")
	}

	// Subsequent release rc3: authoritative live inventory must include rc1+rc2.
	workRC3 := t.TempDir()
	cmd3 := exec.Command("bash", prepare, srv.URL, workRC3)
	cmd3.Env = append(os.Environ(), "PAGES_ALLOW_GH_PAGES_BOOTSTRAP=0")
	if out, err := cmd3.CombinedOutput(); err != nil {
		t.Fatalf("prepare rc3 baseline from live: %v\n%s", err, out)
	}
	for _, ver := range []string{"1.0.0-rc1", "1.0.0-rc2"} {
		if _, err := os.Stat(filepath.Join(workRC3, "lab", ver, "manifest.json")); err != nil {
			t.Fatalf("missing lab/%s after live baseline: %v", ver, err)
		}
	}
	unsignedRC3 := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsignedRC3, 0o755)
	bin3 := []byte{0x7f, 'E', 'L', 'F', '3'}
	_ = writeVersionBundle(t, unsignedRC3, pub, "1.0.0-rc3", bin3)
	_ = os.WriteFile(filepath.Join(unsignedRC3, "pve-zfs-monitor-linux-arm64"), bin3, 0o644)
	pubPath3 := filepath.Join(workRC3, "scripts", "trusted-signing-key.pub.b64")
	if out, err := exec.Command("bash", merge, workRC3, unsignedRC3, pubPath3, root, "1.0.0-rc3").CombinedOutput(); err != nil {
		t.Fatalf("merge rc3: %v\n%s", err, out)
	}
	deployRC3 := t.TempDir()
	if out, err := exec.Command("bash", materialize, workRC3, deployRC3).CombinedOutput(); err != nil {
		t.Fatalf("materialize rc3: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", writeManifest, deployRC3).CombinedOutput(); err != nil {
		t.Fatalf("write manifest rc3: %v\n%s", err, out)
	}
	if out, err := exec.Command("bash", verifyInv, deployRC3).CombinedOutput(); err != nil {
		t.Fatalf("verify inventory rc3: %v\n%s", err, out)
	}
	assertNoForbiddenDeploymentPaths(t, deployRC3)
	srv3 := httptest.NewServer(http.FileServer(http.Dir(deployRC3)))
	defer srv3.Close()
	assertInventoryPathsFetchable(t, srv3.URL, deployRC3)
	for _, ver := range []string{"1.0.0-rc1", "1.0.0-rc2", "1.0.0-rc3"} {
		if _, err := os.Stat(filepath.Join(deployRC3, "lab", ver, "pve-zfs-monitor-linux-amd64")); err != nil {
			t.Fatalf("missing binary for %s: %v", ver, err)
		}
	}
	latestRaw, _ := os.ReadFile(filepath.Join(deployRC3, "lab", "latest", "manifest.json"))
	if !strings.Contains(string(latestRaw), "1.0.0-rc3") {
		t.Fatalf("lab/latest must be rc3, got %s", latestRaw)
	}
	stable, err := os.ReadFile(filepath.Join(deployRC3, "stable", "1.0.0", "README.txt"))
	if err != nil || !strings.Contains(string(stable), "stable-channel") {
		t.Fatalf("stable channel must survive successive releases: %v", err)
	}
}

func TestBootstrapRejectedWhenLiveLatestRC2AndInventory404(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))

	ghPagesFrozen := t.TempDir()
	deployed := t.TempDir()
	buildPublicRC1Site(t, pub, ghPagesFrozen, deployed)

	// Simulate Actions-published rc2 on live (latest=rc2) but missing deploy inventory.
	unsignedRC2 := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsignedRC2, 0o755)
	bin2 := []byte{0x7f, 'E', 'L', 'F', '2'}
	_ = writeVersionBundle(t, unsignedRC2, pub, "1.0.0-rc2", bin2)
	_ = os.WriteFile(filepath.Join(unsignedRC2, "pve-zfs-monitor-linux-arm64"), bin2, 0o644)
	pubPath := filepath.Join(deployed, "scripts", "trusted-signing-key.pub.b64")
	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	if out, err := exec.Command("bash", merge, deployed, unsignedRC2, pubPath, root, "1.0.0-rc2").CombinedOutput(); err != nil {
		t.Fatalf("seed live rc2: %v\n%s", err, out)
	}
	deployedSnap, _ := os.ReadFile(filepath.Join(deployed, "lab", "latest", "manifest.json"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pages-deploy-manifest.txt" {
			http.NotFound(w, r)
			return
		}
		http.FileServer(http.Dir(deployed)).ServeHTTP(w, r)
	}))
	defer srv.Close()

	prepare := filepath.Join(root, "release", "signing", "prepare-pages-site-baseline.sh")
	work := t.TempDir()
	cmd := exec.Command("bash", prepare, srv.URL, work)
	cmd.Env = append(os.Environ(),
		"PAGES_ALLOW_GH_PAGES_BOOTSTRAP=1",
		"PAGES_BOOTSTRAP_SOURCE_DIR="+ghPagesFrozen,
	)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected bootstrap rejection when live latest is rc2, out=%s", out)
	}
	if !strings.Contains(string(out), "refusing gh-pages bootstrap") {
		t.Fatalf("expected bootstrap guard message, out=%s", out)
	}
	got, _ := os.ReadFile(filepath.Join(deployed, "lab", "latest", "manifest.json"))
	if !strings.Contains(string(got), "1.0.0-rc2") || string(got) != string(deployedSnap) {
		t.Fatal("live deployment must remain unchanged after failed rc3 baseline")
	}
}

func TestLiveBaselineFetchFailsOnIncompleteInventory(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/pages-deploy-manifest.txt":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("index.html\nlab/missing-version/manifest.json\n"))
		default:
			if r.URL.Path == "/index.html" {
				_, _ = w.Write([]byte("ok"))
				return
			}
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	fetch := filepath.Join(root, "release", "signing", "fetch-existing-pages-site.sh")
	outDir := t.TempDir()
	cmd := exec.Command("bash", fetch, srv.URL, outDir)
	cmd.Env = append(os.Environ(), "PAGES_FETCH_MODE=live", "PAGES_LIVE_STRICT=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected incomplete inventory fetch to fail, out=%s", out)
	}
}

func buildPublicRC1Site(t *testing.T, pub ed25519.PublicKey, ghPagesFrozen, deployed string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(ghPagesFrozen, ".git"), 0o755)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, ".git", "HEAD"), []byte("ref: refs/heads/gh-pages\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(ghPagesFrozen, "cmd", "pve-zfs-monitor"), 0o755)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "cmd", "pve-zfs-monitor", "main.go"), []byte("package main\n"), 0o644)
	_ = os.MkdirAll(filepath.Join(ghPagesFrozen, "stable", "1.0.0"), 0o755)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "stable", "1.0.0", "README.txt"), []byte("stable-channel"), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "index.html"), []byte("homepage"), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, ".nojekyll"), nil, 0o644)
	_ = os.MkdirAll(filepath.Join(ghPagesFrozen, "scripts"), 0o755)
	pubPath := filepath.Join(ghPagesFrozen, "scripts", "trusted-signing-key.pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "scripts", "install.sh"), []byte("#!/bin/sh\n"), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "scripts", "install.sh.sha256"), []byte("x  install.sh\n"), 0o644)

	rc1Dir := filepath.Join(ghPagesFrozen, "lab", "1.0.0-rc1")
	_ = os.MkdirAll(rc1Dir, 0o755)
	bin1 := []byte{0x7f, 'E', 'L', 'F', '1'}
	_ = writeVersionBundle(t, rc1Dir, pub, "1.0.0-rc1", bin1)
	_ = os.WriteFile(filepath.Join(rc1Dir, "pve-zfs-monitor-linux-arm64"), bin1, 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "lab", "published-versions.txt"), []byte("1.0.0-rc1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "lab", "published-versions.txt"), []byte("1.0.0-rc1\n"), 0o644)
	_ = os.WriteFile(filepath.Join(ghPagesFrozen, "lab", ".published-versions"), []byte("1.0.0-rc1\n"), 0o644)
	latest := filepath.Join(ghPagesFrozen, "lab", "latest")
	_ = os.MkdirAll(latest, 0o755)
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json"), filepath.Join(latest, "manifest.json"))
	_ = copyFile(filepath.Join(rc1Dir, "manifest.json.sig.json"), filepath.Join(latest, "manifest.json.sig.json"))

	root := filepath.Clean(filepath.Join("..", ".."))
	materialize := filepath.Join(root, "release", "signing", "materialize-public-pages-site.sh")
	if out, err := exec.Command("bash", materialize, ghPagesFrozen, deployed).CombinedOutput(); err != nil {
		t.Fatalf("materialize public rc1: %v\n%s", err, out)
	}
}

func resetDir(t *testing.T, dst, src string) {
	t.Helper()
	entries, _ := os.ReadDir(dst)
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dst, e.Name()))
	}
	ents, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		from := filepath.Join(src, e.Name())
		to := filepath.Join(dst, e.Name())
		if err := copyTree(from, to); err != nil {
			t.Fatal(err)
		}
	}
}

func copyTree(src, dst string) error {
	return exec.Command("cp", "-a", src, dst).Run()
}

func assertInventoryPathsFetchable(t *testing.T, baseURL, site string) {
	t.Helper()
	root := filepath.Clean(filepath.Join("..", ".."))
	script := filepath.Join(root, "release", "signing", "verify-pages-inventory-urls-fetchable.sh")
	manifest := filepath.Join(site, "pages-deploy-manifest.txt")
	if out, err := exec.Command("bash", script, baseURL, manifest).CombinedOutput(); err != nil {
		t.Fatalf("inventory fetchable check: %v\n%s", err, out)
	}
	manRaw, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	body := string(manRaw)
	if !strings.Contains(body, ".nojekyll") {
		t.Fatalf("inventory must include .nojekyll:\n%s", body)
	}
	if !strings.Contains(body, "lab/published-versions.txt") {
		t.Fatalf("inventory must include lab/published-versions.txt:\n%s", body)
	}
}

func assertNoForbiddenDeploymentPaths(t *testing.T, site string) {
	t.Helper()
	forbidden := []string{".git", "cmd", "go.mod"}
	for _, frag := range forbidden {
		if _, err := os.Stat(filepath.Join(site, frag)); err == nil {
			t.Fatalf("forbidden path %q present in deployment", frag)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(site, "pages-deploy-manifest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifest), ".git/") || strings.Contains(string(manifest), "cmd/") {
		t.Fatalf("manifest must not list VCS/repo internals:\n%s", manifest)
	}
}
