package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergePreservesStableChannelAndExtraPublicFiles(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	pub, _ := testSigningKeypair()
	root := filepath.Clean(filepath.Join("..", ".."))
	site := t.TempDir()

	_ = os.MkdirAll(filepath.Join(site, "stable", "1.0.0"), 0o755)
	_ = os.WriteFile(filepath.Join(site, "stable", "1.0.0", "README.txt"), []byte("stable-channel"), 0o644)
	_ = os.WriteFile(filepath.Join(site, "index.html"), []byte("homepage"), 0o644)
	_ = os.WriteFile(filepath.Join(site, ".nojekyll"), nil, 0o644)
	_ = os.MkdirAll(filepath.Join(site, "scripts"), 0o755)
	pubPath := filepath.Join(site, "scripts", "trusted-signing-key.pub.b64")
	_ = os.WriteFile(pubPath, []byte(testPubB64), 0o644)
	_ = os.WriteFile(filepath.Join(site, "scripts", "install.sh"), []byte("#!/bin/sh\n"), 0o644)
	_ = os.WriteFile(filepath.Join(site, "scripts", "install.sh.sha256"), []byte("x  install.sh\n"), 0o644)

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

	unsigned := filepath.Join(t.TempDir(), "unsigned")
	_ = os.MkdirAll(unsigned, 0o755)
	bin2 := []byte{0x7f, 'E', 'L', 'F', '2'}
	_ = writeVersionBundle(t, unsigned, pub, "1.0.0-rc2", bin2)
	_ = os.WriteFile(filepath.Join(unsigned, "pve-zfs-monitor-linux-arm64"), bin2, 0o644)

	merge := filepath.Join(root, "release", "signing", "merge-pages-for-deploy.sh")
	out, err := exec.Command("bash", merge, site, unsigned, pubPath, root, "1.0.0-rc2").CombinedOutput()
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}

	stable, err := os.ReadFile(filepath.Join(site, "stable", "1.0.0", "README.txt"))
	if err != nil || !strings.Contains(string(stable), "stable-channel") {
		t.Fatalf("stable channel must survive merge: %v", err)
	}
	home, _ := os.ReadFile(filepath.Join(site, "index.html"))
	if !strings.Contains(string(home), "homepage") {
		t.Fatal("homepage must survive merge")
	}

	writeManifest := filepath.Join(root, "release", "signing", "write-pages-deploy-manifest.sh")
	if out, err := exec.Command("bash", writeManifest, site).CombinedOutput(); err != nil {
		t.Fatalf("write manifest: %v\n%s", err, out)
	}
	manifest, err := os.ReadFile(filepath.Join(site, "pages-deploy-manifest.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "stable/1.0.0/README.txt") {
		t.Fatalf("deploy manifest must list stable assets:\n%s", manifest)
	}
}

func TestFetchPagesSiteFromGitPreservesGhPagesTree(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash required")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	outDir := t.TempDir()
	fetch := filepath.Join(root, "release", "signing", "fetch-existing-pages-site.sh")
	cmd := exec.Command("bash", fetch, "", outDir)
	cmd.Env = append(os.Environ(),
		"PAGES_FETCH_MODE=git",
		"PAGES_GIT_REPO="+root,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git fetch: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(outDir, "lab", "1.0.0-rc1", "manifest.json")); err != nil {
		t.Fatalf("expected rc1 from gh-pages: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "index.html")); err != nil {
		t.Fatal("expected index.html from gh-pages")
	}
}
